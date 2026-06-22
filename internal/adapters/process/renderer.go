package process

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/3-lines-studio/bifrost/internal/core"
)

const (
	renderTimeout = 30 * time.Second
	buildTimeout  = 120 * time.Second
	socketTimeout = 10 * time.Second
)

var (
	//go:embed react_runtime.ts
	ReactRuntimeSource string

	//go:embed react_compiler_plugin.ts
	reactCompilerPluginSource string

	//go:embed svelte_plugin.ts
	sveltePluginSource string
)

func RuntimeSource(mode core.Mode, frameworks ...core.Framework) string {
	tailwindPlugin := `(await import("bun-plugin-tailwind")).default`
	if mode == core.ModeProd {
		tailwindPlugin = "undefined"
	}
	hasReact := false
	hasSvelte := false
	for _, fw := range frameworks {
		if fw == core.FrameworkSvelte {
			hasSvelte = true
		} else {
			hasReact = true
		}
	}
	reactCompilerPlugin := "undefined"
	sveltePlugin := "undefined"
	if hasReact {
		reactCompilerPlugin = strings.TrimSpace(reactCompilerPluginSource)
	}
	if hasSvelte {
		sveltePlugin = strings.TrimSpace(sveltePluginSource)
	}
	src := strings.ReplaceAll(ReactRuntimeSource, "BIFROST_TAILWIND_PLUGIN", tailwindPlugin)
	src = strings.ReplaceAll(src, "BIFROST_REACT_COMPILER_PLUGIN", reactCompilerPlugin)
	src = strings.ReplaceAll(src, "BIFROST_SVELTE_PLUGIN", sveltePlugin)
	return src
}

type Renderer struct {
	cmd     *exec.Cmd
	socket  string
	client  *http.Client
	cleanup func()
}

type rendererProcessConfig struct {
	command []string
	cwd     string
	source  string
	env     []string
	cleanup func()
}

type renderRequestPayload struct {
	Path  string `json:"path"`
	Props any    `json:"props"`
}

func uniqueSocketPath() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	id := hex.EncodeToString(b[:])
	return filepath.Join(os.TempDir(), fmt.Sprintf("bifrost-%d-%s.sock", os.Getpid(), id))
}

func removeStaleSocket(path string) {
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		_ = os.Remove(path)
	}
}

func NewRenderer(mode core.Mode, source string, extraEnv ...string) (*Renderer, error) {
	if source == "" {
		source = RuntimeSource(mode, core.FrameworkReact)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	return startRendererProcess(rendererProcessConfig{
		command: []string{"bun", "run", "-"},
		cwd:     cwd,
		source:  source,
		env:     extraEnv,
	})
}

func NewRendererFromExecutable(executablePath string, cleanup func()) (*Renderer, error) {
	return startRendererProcess(rendererProcessConfig{
		command: []string{executablePath},
		cleanup: cleanup,
	})
}

func newUnixTransport(socket string) *http.Transport {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socket)
		},
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
}

func newHTTPClient(socket string) *http.Client {
	return &http.Client{
		Transport: newUnixTransport(socket),
		Timeout:   buildTimeout,
	}
}

func startRendererProcess(cfg rendererProcessConfig) (*Renderer, error) {
	socket := uniqueSocketPath()
	removeStaleSocket(socket)

	cmd := exec.Command(cfg.command[0], cfg.command[1:]...)
	cmd.Dir = cfg.cwd
	cmd.Env = append(os.Environ(), append([]string{"BIFROST_SOCKET=" + socket}, cfg.env...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if cfg.source != "" {
		cmd.Stdin = strings.NewReader(cfg.source)
	}

	if err := cmd.Start(); err != nil {
		if cfg.cleanup != nil {
			cfg.cleanup()
		}
		return nil, fmt.Errorf("failed to start runtime process: %w", err)
	}

	if err := waitForStartedSocket(cmd, socket, cfg.cleanup); err != nil {
		return nil, err
	}

	return &Renderer{
		cmd:     cmd,
		socket:  socket,
		client:  newHTTPClient(socket),
		cleanup: cfg.cleanup,
	}, nil
}

func waitForStartedSocket(cmd *exec.Cmd, socket string, cleanup func()) error {
	if err := waitForSocket(socket, socketTimeout); err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = os.Remove(socket)
		if cleanup != nil {
			cleanup()
		}
		return err
	}
	return nil
}

func (r *Renderer) Stop() error {
	if r.cmd == nil || r.cmd.Process == nil {
		if r.cleanup != nil {
			r.cleanup()
		}
		return nil
	}
	err := r.cmd.Process.Kill()
	_, _ = r.cmd.Process.Wait()
	_ = os.Remove(r.socket)
	if r.cleanup != nil {
		r.cleanup()
	}
	return err
}

type errorPositionJSON struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	LineText string `json:"lineText"`
}

type errorDetailJSON struct {
	Message   string             `json:"message"`
	Stack     string             `json:"stack"`
	Position  *errorPositionJSON `json:"position"`
	Specifier string             `json:"specifier"`
	Referrer  string             `json:"referrer"`
}

type bunErrorJSON struct {
	Message string            `json:"message"`
	Stack   string            `json:"stack"`
	Errors  []errorDetailJSON `json:"errors"`
}

func bunErrorToStructured(e *bunErrorJSON, errorType string) *core.StructuredError {
	if e == nil {
		return nil
	}
	se := &core.StructuredError{
		ErrorType: errorType,
		Message:   e.Message,
		Stack:     e.Stack,
	}
	for _, detail := range e.Errors {
		sub := core.StructuredError{
			Message:   detail.Message,
			Stack:     detail.Stack,
			Specifier: detail.Specifier,
			Referrer:  detail.Referrer,
		}
		if detail.Position != nil {
			sub.File = detail.Position.File
			sub.Line = detail.Position.Line
			sub.Column = detail.Position.Column
			sub.LineText = detail.Position.LineText
		}
		se.SubErrors = append(se.SubErrors, sub)
	}
	if len(se.SubErrors) > 0 && se.File == "" {
		first := se.SubErrors[0]
		se.File = first.File
		se.Line = first.Line
		se.Column = first.Column
		se.LineText = first.LineText
		se.Specifier = first.Specifier
		se.Referrer = first.Referrer
	}
	return se
}

func formatRenderError(e *bunErrorJSON) *core.StructuredError {
	return bunErrorToStructured(e, "Render Error")
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// MarshalRenderRequestJSON builds the JSON body for POST /render (exported for tests).
func MarshalRenderRequestJSON(path string, props any) ([]byte, error) {
	return json.Marshal(renderRequestPayload{
		Path:  path,
		Props: props,
	})
}

func (r *Renderer) postRender(ctx context.Context, path string, props any) (*http.Response, error) {
	jsonBody, err := MarshalRenderRequestJSON(path, props)
	if err != nil {
		return nil, err
	}
	req, err := newJSONRequest(ctx, "/render", jsonBody)
	if err != nil {
		return nil, err
	}
	return r.client.Do(req)
}

func newJSONRequest(ctx context.Context, endpoint string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost"+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// renderFromDecoder consumes Bun /render output: one legacy JSON object or two NDJSON lines (head then html).
func renderFromDecoder(dec *json.Decoder) (head, html string, err error) {
	type firstMsg struct {
		Error *bunErrorJSON `json:"error"`
		Head  *string       `json:"head"`
		HTML  *string       `json:"html"`
	}

	var first firstMsg
	if err := dec.Decode(&first); err != nil {
		return "", "", fmt.Errorf("render response: %w", err)
	}
	if first.Error != nil {
		se := formatRenderError(first.Error)
		if se != nil {
			return "", "", se
		}
		return "", "", fmt.Errorf("render error")
	}

	if first.HTML != nil {
		return derefString(first.Head), *first.HTML, nil
	}

	head = derefString(first.Head)

	var second struct {
		Error *bunErrorJSON `json:"error"`
		HTML  *string       `json:"html"`
	}
	if err := dec.Decode(&second); err != nil {
		return "", "", fmt.Errorf("render body: %w", err)
	}
	if second.Error != nil {
		se := formatRenderError(second.Error)
		if se != nil {
			return "", "", se
		}
		return "", "", fmt.Errorf("render error")
	}
	if second.HTML == nil {
		return "", "", fmt.Errorf("missing html in render response")
	}
	return head, *second.HTML, nil
}

func (r *Renderer) Render(path string, props any) (core.RenderedPage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), renderTimeout)
	defer cancel()

	resp, err := r.postRender(ctx, path, props)
	if err != nil {
		return core.RenderedPage{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	head, body, err := renderFromDecoder(json.NewDecoder(resp.Body))
	if err != nil {
		return core.RenderedPage{}, err
	}
	return core.RenderedPage{Head: head, Body: body}, nil
}

func (r *Renderer) Build(entrypoints []string, outdir string, entryNames []string, framework string) (map[string]core.ClientBuildResult, error) {
	if len(entrypoints) == 0 {
		return nil, fmt.Errorf("missing entrypoints")
	}

	if outdir == "" {
		return nil, fmt.Errorf("missing outdir")
	}

	if len(entryNames) != len(entrypoints) {
		return nil, fmt.Errorf("entryNames length %d does not match entrypoints length %d", len(entryNames), len(entrypoints))
	}

	ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
	defer cancel()

	reqBody := map[string]any{
		"entrypoints": entrypoints,
		"outdir":      outdir,
		"entryNames":  entryNames,
		"framework":   framework,
	}

	var result struct {
		OK      bool                              `json:"ok"`
		Entries map[string]core.ClientBuildResult `json:"entries"`
		Error   *bunErrorJSON                     `json:"error"`
	}

	if err := r.postJSON(ctx, "/build", reqBody, &result); err != nil {
		return nil, err
	}

	if result.Error != nil {
		se := bunErrorToStructured(result.Error, "Build Error")
		if se != nil {
			return nil, fmt.Errorf("build failed: %w", se)
		}
		return nil, fmt.Errorf("build failed: %s", result.Error.Message)
	}

	if !result.OK {
		return nil, fmt.Errorf("build failed for entrypoints %v -> %s", entrypoints, outdir)
	}

	if result.Entries == nil {
		return nil, fmt.Errorf("build returned no entries")
	}

	out := make(map[string]core.ClientBuildResult, len(entryNames))
	for _, name := range entryNames {
		built, ok := result.Entries[name]
		if !ok {
			return nil, fmt.Errorf("missing build result for entry %q", name)
		}
		if built.Script == "" {
			built = core.ClientBuildResult{
				Script: "/dist/" + name + ".js",
				CSS:    "/dist/" + name + ".css",
			}
		}
		out[name] = built
	}
	return out, nil
}

func (r *Renderer) BuildSSR(entrypoints []string, outdir string, framework string) error {
	if len(entrypoints) == 0 {
		return fmt.Errorf("missing entrypoints")
	}

	if outdir == "" {
		return fmt.Errorf("missing outdir")
	}

	ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
	defer cancel()

	reqBody := map[string]any{
		"entrypoints": entrypoints,
		"outdir":      outdir,
		"target":      "bun",
		"framework":   framework,
	}

	var result struct {
		OK    bool          `json:"ok"`
		Error *bunErrorJSON `json:"error"`
	}

	if err := r.postJSON(ctx, "/build", reqBody, &result); err != nil {
		return err
	}

	if result.Error != nil {
		se := bunErrorToStructured(result.Error, "Build Error")
		if se != nil {
			return fmt.Errorf("ssr build failed: %w", se)
		}
		return fmt.Errorf("ssr build failed: %s", result.Error.Message)
	}

	if !result.OK {
		return fmt.Errorf("ssr build failed for entrypoints %v -> %s", entrypoints, outdir)
	}

	return nil
}

func (r *Renderer) postJSON(ctx context.Context, endpoint string, body any, result any) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := newJSONRequest(ctx, endpoint, jsonBody)
	if err != nil {
		return err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	return json.NewDecoder(resp.Body).Decode(result)
}

func waitForSocket(socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", socketPath, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for bun socket at %s", socketPath)
}
