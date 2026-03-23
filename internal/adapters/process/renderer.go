package process

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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
)

func RuntimeSource(mode core.Mode) string {
	tailwindPlugin := `(await import("bun-plugin-tailwind")).default`
	if mode == core.ModeProd {
		tailwindPlugin = "undefined"
	}
	return strings.ReplaceAll(ReactRuntimeSource, "BIFROST_TAILWIND_PLUGIN", tailwindPlugin)
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
	Path       string         `json:"path"`
	Props      map[string]any `json:"props"`
	StreamBody bool           `json:"streamBody,omitempty"`
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
		source = RuntimeSource(mode)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	return startRendererProcess(rendererProcessConfig{
		command: []string{"bun", "run", "--smol", "-"},
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

type renderErrJSON struct {
	Message string `json:"message"`
	Stack   string `json:"stack"`
	Errors  []struct {
		Message string `json:"message"`
		Stack   string `json:"stack"`
	} `json:"errors"`
}

func formatRenderError(e *renderErrJSON) error {
	if e == nil {
		return nil
	}
	var sb strings.Builder
	sb.WriteString(e.Message)

	if len(e.Errors) > 0 {
		sb.WriteString("\n\nErrors:")
		for i, err := range e.Errors {
			fmt.Fprintf(&sb, "\n  %d. %s", i+1, err.Message)
			if err.Stack != "" {
				fmt.Fprintf(&sb, "\n     Stack: %s", err.Stack)
			}
		}
	}

	if e.Stack != "" {
		fmt.Fprintf(&sb, "\n\nStack:\n%s", e.Stack)
	}

	return fmt.Errorf("%s", sb.String())
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// MarshalRenderRequestJSON builds the JSON body for POST /render (exported for tests).
func MarshalRenderRequestJSON(path string, props map[string]any, streamBody bool) ([]byte, error) {
	return json.Marshal(renderRequestPayload{
		Path:       path,
		Props:      props,
		StreamBody: streamBody,
	})
}

func (r *Renderer) postRender(ctx context.Context, path string, props map[string]any, streamBody bool) (*http.Response, error) {
	jsonBody, err := MarshalRenderRequestJSON(path, props, streamBody)
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

// renderChunkedFromDecoder consumes Bun /render output: one legacy JSON object or two NDJSON lines (head then html).
func renderChunkedFromDecoder(dec *json.Decoder, onHead func(head string) error, onBody func(body string) error) error {
	type firstMsg struct {
		Error *renderErrJSON `json:"error"`
		Head  *string        `json:"head"`
		HTML  *string        `json:"html"`
	}

	var first firstMsg
	if err := dec.Decode(&first); err != nil {
		return fmt.Errorf("render response: %w", err)
	}
	if first.Error != nil {
		return formatRenderError(first.Error)
	}

	if first.HTML != nil {
		head := derefString(first.Head)
		if err := onHead(head); err != nil {
			return err
		}
		return onBody(*first.HTML)
	}

	head := derefString(first.Head)
	if err := onHead(head); err != nil {
		return err
	}

	var second struct {
		Error *renderErrJSON `json:"error"`
		HTML  *string        `json:"html"`
	}
	if err := dec.Decode(&second); err != nil {
		return fmt.Errorf("render stream body: %w", err)
	}
	if second.Error != nil {
		return formatRenderError(second.Error)
	}
	if second.HTML == nil {
		return fmt.Errorf("missing html in streamed render response")
	}
	return onBody(*second.HTML)
}

// RenderChunked calls onHead after the first NDJSON object (head), then onBody after the body.
// Legacy single JSON {"html","head"} invokes onHead then onBody in one round trip.
func (r *Renderer) RenderChunked(ctx context.Context, path string, props map[string]any, onHead func(head string) error, onBody func(body string) error) error {
	resp, err := r.postRender(ctx, path, props, false)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	return renderChunkedFromDecoder(json.NewDecoder(resp.Body), onHead, onBody)
}

type renderFirstLine struct {
	Error *renderErrJSON `json:"error"`
	Head  *string        `json:"head"`
	HTML  *string        `json:"html"`
}

func parseRenderFirstLine(line []byte) (head string, html *string, err error) {
	line = bytes.TrimSuffix(line, []byte("\n"))
	if len(line) == 0 {
		return "", nil, fmt.Errorf("empty render response first line")
	}
	var msg renderFirstLine
	if err := json.Unmarshal(line, &msg); err != nil {
		return "", nil, fmt.Errorf("render response first line: %w", err)
	}
	if msg.Error != nil {
		return "", nil, formatRenderError(msg.Error)
	}
	return derefString(msg.Head), msg.HTML, nil
}

func copyResponseBodyWithFlush(dst io.Writer, src io.Reader, flush func(), flushEveryChunk bool) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	flushed := false
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			nw, werr := dst.Write(buf[:n])
			written += int64(nw)
			if flush != nil && (flushEveryChunk || !flushed) {
				flush()
				flushed = true
			}
			if werr != nil {
				return written, werr
			}
			if nw != n {
				return written, io.ErrShortWrite
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			return written, rerr
		}
	}
	return written, nil
}

// RenderBodyStream requests streamBody rendering: first line is JSON {"head"} or {"head","html"} fallback;
// remaining bytes are raw HTML from renderToReadableStream. Writes body HTML to w (not the document suffix).
func (r *Renderer) RenderBodyStream(ctx context.Context, path string, props map[string]any, w io.Writer, flush func(), onHead func(head string) error) error {
	resp, err := r.postRender(ctx, path, props, true)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	br := bufio.NewReader(resp.Body)
	line, err := br.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("render stream: read first line: %w", err)
	}
	head, htmlInLine, err := parseRenderFirstLine(line)
	if err != nil {
		return err
	}
	if htmlInLine != nil {
		if err := onHead(head); err != nil {
			return err
		}
		if _, err := io.WriteString(w, *htmlInLine); err != nil {
			return err
		}
		if flush != nil {
			flush()
		}
		return nil
	}
	if err := onHead(head); err != nil {
		return err
	}
	_, err = copyResponseBodyWithFlush(w, br, flush, false)
	return err
}

func (r *Renderer) Render(path string, props map[string]any) (core.RenderedPage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), renderTimeout)
	defer cancel()

	var page core.RenderedPage
	err := r.RenderChunked(ctx, path, props,
		func(head string) error {
			page.Head = head
			return nil
		},
		func(body string) error {
			page.Body = body
			return nil
		},
	)
	if err != nil {
		return core.RenderedPage{}, err
	}
	return page, nil
}

func (r *Renderer) Build(entrypoints []string, outdir string, entryNames []string) (map[string]core.ClientBuildResult, error) {
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
	}

	var result struct {
		OK      bool                              `json:"ok"`
		Entries map[string]core.ClientBuildResult `json:"entries"`
		Error   *struct {
			Message string `json:"message"`
			Stack   string `json:"stack"`
			Errors  []struct {
				Message   string `json:"message"`
				File      string `json:"file"`
				Line      int    `json:"line"`
				Column    int    `json:"column"`
				LineText  string `json:"lineText"`
				Specifier string `json:"specifier"`
				Referrer  string `json:"referrer"`
			} `json:"errors"`
		} `json:"error"`
	}

	if err := r.postJSON(ctx, "/build", reqBody, &result); err != nil {
		return nil, err
	}

	if result.Error != nil {
		var errorDetails strings.Builder
		errorDetails.WriteString(result.Error.Message)
		if len(result.Error.Errors) > 0 {
			errorDetails.WriteString("\n")
			for _, e := range result.Error.Errors {
				_, _ = fmt.Fprintf(&errorDetails, "  - %s", e.Message)
				if e.File != "" {
					_, _ = fmt.Fprintf(&errorDetails, " (%s:%d:%d)", e.File, e.Line, e.Column)
				}
				errorDetails.WriteString("\n")
			}
		}
		return nil, fmt.Errorf("build failed: %s", errorDetails.String())
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

func (r *Renderer) BuildSSR(entrypoints []string, outdir string) error {
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
	}

	var result struct {
		OK    bool `json:"ok"`
		Error *struct {
			Message string `json:"message"`
			Stack   string `json:"stack"`
			Errors  []struct {
				Message   string `json:"message"`
				File      string `json:"file"`
				Line      int    `json:"line"`
				Column    int    `json:"column"`
				LineText  string `json:"lineText"`
				Specifier string `json:"specifier"`
				Referrer  string `json:"referrer"`
			} `json:"errors"`
		} `json:"error"`
	}

	if err := r.postJSON(ctx, "/build", reqBody, &result); err != nil {
		return err
	}

	if result.Error != nil {
		var errorDetails strings.Builder
		errorDetails.WriteString(result.Error.Message)
		if len(result.Error.Errors) > 0 {
			errorDetails.WriteString("\n")
			for _, e := range result.Error.Errors {
				_, _ = fmt.Fprintf(&errorDetails, "  - %s", e.Message)
				if e.File != "" {
					_, _ = fmt.Fprintf(&errorDetails, " (%s:%d:%d)", e.File, e.Line, e.Column)
				}
				errorDetails.WriteString("\n")
			}
		}
		return fmt.Errorf("ssr build failed: %s", errorDetails.String())
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
