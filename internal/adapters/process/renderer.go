package process

import (
	"crypto/rand"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/3-lines-studio/bifrost/internal/core"
)

const (
	renderTimeout = 30 * time.Second
	buildTimeout  = 120 * time.Second
	socketTimeout = 10 * time.Second

	connPoolSize = 16
)

// Frame kinds on the renderer unix socket.
const (
	frameKindRender = 0
	frameKindError  = 1
	frameKindBuild  = 2
)

var (
	//go:embed react_runtime.ts
	ReactRuntimeSource string

	//go:embed react_compiler_plugin.ts
	reactCompilerPluginSource string
)

func RuntimeSource(mode core.Mode, frameworks ...core.Framework) string {
	tailwindPlugin := `(await import("bun-plugin-tailwind")).default`
	if mode == core.ModeProd {
		tailwindPlugin = "undefined"
	}
	reactCompilerPlugin := strings.TrimSpace(reactCompilerPluginSource)
	src := strings.ReplaceAll(ReactRuntimeSource, "BIFROST_TAILWIND_PLUGIN", tailwindPlugin)
	src = strings.ReplaceAll(src, "BIFROST_REACT_COMPILER_PLUGIN", reactCompilerPlugin)
	return src
}

type Renderer struct {
	cmd     *exec.Cmd
	socket  string
	conns   chan *rendererConn
	cleanup func()
}

type rendererConn struct {
	net.Conn
	frameReader
	// broken marks a conn that must not be reused, e.g. after a stream path
	// error left unread frame bytes on the wire.
	broken bool
}

// frameReader decodes length-prefixed frames. The scratch buffer is reused
// across frames so large response bodies stop allocating after the first read.
type frameReader struct {
	r       io.Reader
	scratch []byte
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

func newRendererConn(socket string) (*rendererConn, error) {
	conn, err := net.DialTimeout("unix", socket, 5*time.Second)
	if err != nil {
		return nil, err
	}
	return &rendererConn{Conn: conn, frameReader: frameReader{r: conn}}, nil
}

func (fr *frameReader) readByte() (byte, error) {
	var b [1]byte
	if _, err := io.ReadFull(fr.r, b[:]); err != nil {
		return 0, err
	}
	return b[0], nil
}

func (fr *frameReader) readLen() (int, error) {
	var b [4]byte
	if _, err := io.ReadFull(fr.r, b[:]); err != nil {
		return 0, err
	}
	return int(binary.BigEndian.Uint32(b[:])), nil
}

func (fr *frameReader) readBytes() ([]byte, error) {
	n, err := fr.readLen()
	if err != nil {
		return nil, err
	}
	if n > len(fr.scratch) {
		fr.scratch = make([]byte, n)
	}
	if _, err := io.ReadFull(fr.r, fr.scratch[:n]); err != nil {
		return nil, err
	}
	return fr.scratch[:n], nil
}

func (fr *frameReader) readString() (string, error) {
	b, err := fr.readBytes()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (fr *frameReader) readError() error {
	b, err := fr.readBytes()
	if err != nil {
		return err
	}
	var be bunErrorJSON
	if err := json.Unmarshal(b, &be); err != nil {
		return fmt.Errorf("render error: %s", string(b))
	}
	se := formatRenderError(&be)
	if se != nil {
		return se
	}
	return fmt.Errorf("render error")
}

func writeFrame(w io.Writer, kind byte, payload []byte) error {
	var header [5]byte
	header[0] = kind
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	if len(payload) > 0 {
		_, err := w.Write(payload)
		return err
	}
	return nil
}

func isStaleConn(err error) bool {
	return errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET)
}

func isBrokenConn(err error) bool {
	if err == nil {
		return false
	}
	if isStaleConn(err) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func (r *Renderer) withConn(timeout time.Duration, fn func(*rendererConn) error) error {
	conn := <-r.conns
	if conn.Conn == nil {
		nc, err := newRendererConn(r.socket)
		if err != nil {
			r.conns <- conn
			return err
		}
		conn = nc
	}

	_ = conn.SetDeadline(time.Now().Add(timeout))
	err := fn(conn)
	_ = conn.SetDeadline(time.Time{})
	if isBrokenConn(err) || conn.broken {
		_ = conn.Close()
		conn.Conn = nil
	}
	r.conns <- conn
	return err
}

// retryStale re-runs a transaction once when the pooled connection went stale,
// e.g. after a long idle period. Only call for transactions without side effects
// visible to the caller (buffered reads, builds) — never for streamed output.
// A retried build re-runs Bun.build to the same outdir; recovery, not idempotence.
func (r *Renderer) retryStale(timeout time.Duration, fn func(*rendererConn) error) error {
	err := r.withConn(timeout, fn)
	if isStaleConn(err) {
		err = r.withConn(timeout, fn)
	}
	return err
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

	conns := make(chan *rendererConn, connPoolSize)
	for i := 0; i < connPoolSize; i++ {
		conns <- &rendererConn{}
	}

	return &Renderer{
		cmd:     cmd,
		socket:  socket,
		conns:   conns,
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

// MarshalRenderRequestJSON builds the JSON body for a render request (exported for tests).
func MarshalRenderRequestJSON(path string, props any) ([]byte, error) {
	return json.Marshal(renderRequestPayload{
		Path:  path,
		Props: props,
	})
}

func (r *Renderer) Render(path string, props any) (core.RenderedPage, error) {
	payload, err := MarshalRenderRequestJSON(path, props)
	if err != nil {
		return core.RenderedPage{}, err
	}
	var page core.RenderedPage
	err = r.retryStale(renderTimeout, func(c *rendererConn) error {
		if err := writeFrame(c, frameKindRender, payload); err != nil {
			return err
		}
		kind, err := c.readByte()
		if err != nil {
			return err
		}
		switch kind {
		case frameKindError:
			return c.readError()
		case frameKindRender:
			if page.Head, err = c.readString(); err != nil {
				return err
			}
			page.Body, err = c.readString()
			return err
		default:
			return fmt.Errorf("unexpected render response kind %d", kind)
		}
	})
	if err != nil {
		return core.RenderedPage{}, err
	}
	return page, nil
}

// RenderBodyTo streams the rendered body to w. The head is delivered via onHead
// before any body bytes are written, so callers can emit the HTML preamble first.
func (r *Renderer) RenderBodyTo(w io.Writer, path string, props any, onHead func(head string) error) error {
	payload, err := MarshalRenderRequestJSON(path, props)
	if err != nil {
		return err
	}
	return r.withConn(renderTimeout, func(c *rendererConn) error {
		if err := writeFrame(c, frameKindRender, payload); err != nil {
			return err
		}
		kind, err := c.readByte()
		if err != nil {
			return err
		}
		switch kind {
		case frameKindError:
			return c.readError()
		case frameKindRender:
			head, err := c.readString()
			if err != nil {
				return err
			}
			if err := onHead(head); err != nil {
				c.broken = true
				return err
			}
			bodyLen, err := c.readLen()
			if err != nil {
				return err
			}
			// The render itself is done; draining to a slow client must not
			// trip the render deadline.
			_ = c.SetDeadline(time.Time{})
			if err := copyStreamed(w, c, bodyLen); err != nil {
				c.broken = true
				return err
			}
			return nil
		default:
			return fmt.Errorf("unexpected render response kind %d", kind)
		}
	})
}

var streamBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 32*1024)
		return &b
	},
}

func copyStreamed(w io.Writer, r io.Reader, n int) error {
	buf := streamBufPool.Get().(*[]byte)
	defer streamBufPool.Put(buf)
	_, err := io.CopyBuffer(w, io.LimitReader(r, int64(n)), *buf)
	return err
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

	if err := r.buildExchange(reqBody, &result); err != nil {
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

	if err := r.buildExchange(reqBody, &result); err != nil {
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

func (r *Renderer) buildExchange(reqBody any, result any) error {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	return r.retryStale(buildTimeout, func(c *rendererConn) error {
		if err := writeFrame(c, frameKindBuild, payload); err != nil {
			return err
		}
		kind, err := c.readByte()
		if err != nil {
			return err
		}
		switch kind {
		case frameKindBuild:
			b, err := c.readBytes()
			if err != nil {
				return err
			}
			return json.Unmarshal(b, result)
		case frameKindError:
			return c.readError()
		default:
			return fmt.Errorf("unexpected build response kind %d", kind)
		}
	})
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
