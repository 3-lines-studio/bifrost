package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/3-lines-studio/bifrost/internal/types"
)

type Client struct {
	cmd     *exec.Cmd
	socket  string
	client  *http.Client
	cleanup func()
}

func NewClient() (*Client, error) {
	socket := filepath.Join(os.TempDir(), fmt.Sprintf("bifrost-%d.sock", os.Getpid()))

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	source := BunRendererProdSource
	if IsDev() {
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
		cmd.Process.Kill()
		return nil, err
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("unix", socket)
		},
	}

	return &Client{
		cmd:    cmd,
		socket: socket,
		client: &http.Client{Transport: transport},
	}, nil
}

func (c *Client) Stop() error {
	err := c.cmd.Process.Kill()
	if c.cleanup != nil {
		c.cleanup()
	}
	return err
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

type renderResponse struct {
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

func (c *Client) Render(path string, props map[string]any) (types.RenderedPage, error) {
	reqBody := map[string]any{
		"path":  path,
		"props": props,
	}

	var result renderResponse
	if err := c.postJSON("/render", reqBody, &result); err != nil {
		return types.RenderedPage{}, err
	}

	if result.Error != nil {
		var sb strings.Builder
		sb.WriteString(result.Error.Message)

		if len(result.Error.Errors) > 0 {
			sb.WriteString("\n\nErrors:")
			for i, err := range result.Error.Errors {
				sb.WriteString(fmt.Sprintf("\n  %d. %s", i+1, err.Message))
				if err.Stack != "" {
					sb.WriteString(fmt.Sprintf("\n     Stack: %s", err.Stack))
				}
			}
		}

		if result.Error.Stack != "" {
			sb.WriteString(fmt.Sprintf("\n\nStack:\n%s", result.Error.Stack))
		}

		return types.RenderedPage{}, fmt.Errorf("%s", sb.String())
	}

	return types.RenderedPage{
		Body: result.HTML,
		Head: result.Head,
	}, nil
}

type errorPosition struct {
	LineText string `json:"lineText"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
}

type buildResponse struct {
	OK    bool `json:"ok"`
	Error *struct {
		Message string `json:"message"`
		Stack   string `json:"stack"`
		Errors  []struct {
			Message   string        `json:"message"`
			Position  errorPosition `json:"position"`
			Specifier string        `json:"specifier"`
			Referrer  string        `json:"referrer"`
		} `json:"errors"`
	} `json:"error"`
}

type ErrorDetail struct {
	Message   string
	File      string
	Line      int
	Column    int
	LineText  string
	Specifier string
	Referrer  string
}

type BifrostError struct {
	Message string
	Stack   string
	Errors  []ErrorDetail
}

func (e *BifrostError) Error() string {
	return e.Message
}

func (c *Client) Build(entrypoints []string, outdir string) error {
	if len(entrypoints) == 0 {
		return fmt.Errorf("missing entrypoints")
	}

	if outdir == "" {
		return fmt.Errorf("missing outdir")
	}

	reqBody := map[string]any{
		"entrypoints": entrypoints,
		"outdir":      outdir,
	}

	var result buildResponse
	if err := c.postJSON("/build", reqBody, &result); err != nil {
		return err
	}

	if result.Error != nil {
		errors := make([]ErrorDetail, len(result.Error.Errors))
		for i, err := range result.Error.Errors {
			errors[i] = ErrorDetail{
				Message:   err.Message,
				File:      err.Position.File,
				Line:      err.Position.Line,
				Column:    err.Position.Column,
				LineText:  err.Position.LineText,
				Specifier: err.Specifier,
				Referrer:  err.Referrer,
			}
		}
		return &BifrostError{
			Message: result.Error.Message,
			Stack:   result.Error.Stack,
			Errors:  errors,
		}
	}

	if !result.OK {
		return fmt.Errorf("build failed for entrypoints %v -> %s", entrypoints, outdir)
	}

	return nil
}

func (c *Client) BuildWithTarget(entrypoints []string, outdir string, target string) error {
	if len(entrypoints) == 0 {
		return fmt.Errorf("missing entrypoints")
	}

	if outdir == "" {
		return fmt.Errorf("missing outdir")
	}

	reqBody := map[string]any{
		"entrypoints": entrypoints,
		"outdir":      outdir,
		"target":      target,
	}

	var result buildResponse
	if err := c.postJSON("/build", reqBody, &result); err != nil {
		return err
	}

	if result.Error != nil {
		errors := make([]ErrorDetail, len(result.Error.Errors))
		for i, err := range result.Error.Errors {
			errors[i] = ErrorDetail{
				Message:   err.Message,
				File:      err.Position.File,
				Line:      err.Position.Line,
				Column:    err.Position.Column,
				LineText:  err.Position.LineText,
				Specifier: err.Specifier,
				Referrer:  err.Referrer,
			}
		}
		return &BifrostError{
			Message: result.Error.Message,
			Stack:   result.Error.Stack,
			Errors:  errors,
		}
	}

	if !result.OK {
		return fmt.Errorf("build failed for entrypoints %v -> %s", entrypoints, outdir)
	}

	return nil
}

func (c *Client) postJSON(endpoint string, body interface{}, result interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", "http://localhost"+endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return json.NewDecoder(resp.Body).Decode(result)
}

func NewClientFromExecutable(executablePath string, cleanup func()) (*Client, error) {
	socket := filepath.Join(os.TempDir(), fmt.Sprintf("bifrost-%d.sock", os.Getpid()))

	cmd := exec.Command(executablePath)
	cmd.Env = append(os.Environ(), "BIFROST_SOCKET="+socket)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start embedded runtime: %w", err)
	}

	if err := waitForSocket(socket, 5*time.Second); err != nil {
		cmd.Process.Kill()
		return nil, err
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("unix", socket)
		},
	}

	return &Client{
		cmd:     cmd,
		socket:  socket,
		client:  &http.Client{Transport: transport},
		cleanup: cleanup,
	}, nil
}
