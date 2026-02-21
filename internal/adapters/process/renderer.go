package process

import (
	"bytes"
	"context"
	_ "embed"
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

func NewRenderer(mode core.Mode) (*Renderer, error) {
	socket := filepath.Join(os.TempDir(), fmt.Sprintf("bifrost-%d.sock", os.Getpid()))

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	source := BunRendererProdSource
	if mode == core.ModeDev {
		source = BunRendererDevSource
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

	if err := waitForSocket(socket, 5*time.Second); err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("unix", socket)
		},
	}

	return &Renderer{
		cmd:    cmd,
		socket: socket,
		client: &http.Client{Transport: transport},
	}, nil
}

func NewRendererFromExecutable(executablePath string, cleanup func()) (*Renderer, error) {
	socket := filepath.Join(os.TempDir(), fmt.Sprintf("bifrost-%d.sock", os.Getpid()))

	cmd := exec.Command(executablePath)
	cmd.Env = append(os.Environ(), "BIFROST_SOCKET="+socket)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start embedded runtime: %w", err)
	}

	if err := waitForSocket(socket, 5*time.Second); err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("unix", socket)
		},
	}

	return &Renderer{
		cmd:     cmd,
		socket:  socket,
		client:  &http.Client{Transport: transport},
		cleanup: cleanup,
	}, nil
}

func (r *Renderer) Stop() error {
	err := r.cmd.Process.Kill()
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

	if err := r.postJSON("/render", reqBody, &result); err != nil {
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

	if err := r.postJSON("/build", reqBody, &result); err != nil {
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

	if err := r.postJSON("/build", reqBody, &result); err != nil {
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

func (r *Renderer) postJSON(endpoint string, body interface{}, result interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", "http://localhost"+endpoint, bytes.NewReader(jsonBody))
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

func waitForSocket(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for bun socket at %s", path)
}
