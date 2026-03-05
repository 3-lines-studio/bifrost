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
	//go:embed bun_renderer_dev.ts
	BunRendererDevSource string

	//go:embed bun_renderer_prod.ts
	BunRendererProdSource string
)

type Renderer struct {
	cmd     *exec.Cmd
	socket  string
	client  *http.Client
	cleanup func()
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

func NewRenderer(mode core.Mode, source string) (*Renderer, error) {
	socket := uniqueSocketPath()
	removeStaleSocket(socket)

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	if source == "" {
		source = BunRendererProdSource
		if mode == core.ModeDev {
			source = BunRendererDevSource
		}
	}

	cmd := exec.Command("bun", "run", "--smol", "-")
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "BIFROST_SOCKET="+socket)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = strings.NewReader(source)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start bun: %w", err)
	}

	if err := waitForSocket(socket, socketTimeout); err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return nil, err
	}

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socket)
		},
	}

	return &Renderer{
		cmd:    cmd,
		socket: socket,
		client: &http.Client{Transport: transport, Timeout: buildTimeout},
	}, nil
}

func NewRendererFromExecutable(executablePath string, cleanup func()) (*Renderer, error) {
	socket := uniqueSocketPath()
	removeStaleSocket(socket)

	cmd := exec.Command(executablePath)
	cmd.Env = append(os.Environ(), "BIFROST_SOCKET="+socket)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start embedded runtime: %w", err)
	}

	if err := waitForSocket(socket, socketTimeout); err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return nil, err
	}

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socket)
		},
	}

	return &Renderer{
		cmd:     cmd,
		socket:  socket,
		client:  &http.Client{Transport: transport, Timeout: buildTimeout},
		cleanup: cleanup,
	}, nil
}

func (r *Renderer) Stop() error {
	if r.cmd == nil || r.cmd.Process == nil {
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

func (r *Renderer) Render(path string, props map[string]any) (core.RenderedPage, error) {
	reqBody := map[string]any{
		"path":  path,
		"props": props,
	}

	ctx, cancel := context.WithTimeout(context.Background(), renderTimeout)
	defer cancel()

	var result struct {
		HTML  string `json:"html"`
		Head  string `json:"head"`
		Error *struct {
			Message string `json:"message"`
			Stack   string `json:"stack"`
			Errors  []struct {
				Message string `json:"message"`
				Stack   string `json:"stack"`
			} `json:"errors"`
		} `json:"error"`
	}

	if err := r.postJSON(ctx, "/render", reqBody, &result); err != nil {
		return core.RenderedPage{}, err
	}

	if result.Error != nil {
		var sb strings.Builder
		sb.WriteString(result.Error.Message)

		if len(result.Error.Errors) > 0 {
			sb.WriteString("\n\nErrors:")
			for i, err := range result.Error.Errors {
				fmt.Fprintf(&sb, "\n  %d. %s", i+1, err.Message)
				if err.Stack != "" {
					fmt.Fprintf(&sb, "\n     Stack: %s", err.Stack)
				}
			}
		}

		if result.Error.Stack != "" {
			fmt.Fprintf(&sb, "\n\nStack:\n%s", result.Error.Stack)
		}

		return core.RenderedPage{}, fmt.Errorf("%s", sb.String())
	}

	return core.RenderedPage{
		Body: result.HTML,
		Head: result.Head,
	}, nil
}

func (r *Renderer) Build(entrypoints []string, outdir string, entryNames []string) error {
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
		"entryNames":  entryNames,
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
		return fmt.Errorf("build failed: %s", errorDetails.String())
	}

	if !result.OK {
		return fmt.Errorf("build failed for entrypoints %v -> %s", entrypoints, outdir)
	}

	return nil
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

	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost"+endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	return json.NewDecoder(resp.Body).Decode(result)
}

// waitForSocket probes the socket by attempting a connection rather than
// just checking file existence, so we know Bun is actually listening.
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
