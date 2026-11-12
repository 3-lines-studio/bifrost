//go:build !windows

package renderproc

import (
	"bufio"
	"context"
	"crypto/rand"
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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	startupTimeout    = 10 * time.Second
	maxWireFrameBytes = 16 << 20
	frameHead         = 1
	frameBody         = 2
	frameDone         = 3
	frameError        = 4
)

var emptyPayloadProps = []byte(`{}`)

// UnavailableError reports renderer process or transport failure.
type UnavailableError struct{ Err error }

func (e *UnavailableError) Error() string { return e.Err.Error() }
func (e *UnavailableError) Unwrap() error { return e.Err }

// RemoteError reports an application render failure returned by the renderer.
type RemoteError struct{ Message string }

func (e *RemoteError) Error() string { return e.Message }

// Sink receives ordered renderer output.
type Sink interface {
	Head([]byte) error
	Body([]byte) error
}

type child struct {
	cmd       *exec.Cmd
	waitDone  chan struct{}
	waitMu    sync.Mutex
	waitErr   error
	closeOnce sync.Once
}

type pooledConn struct {
	conn   net.Conn
	reader *bufio.Reader
}

// Process is one long-lived renderer executable reached over a Unix socket with
// length-prefixed frames. Each connection carries one render at a time, so
// concurrent renders dial extra connections and reuse them afterwards.
type Process struct {
	*child
	socket string

	mu   sync.Mutex
	idle []*pooledConn
}

const maxIdleConns = 8

func Start(executable, workDir string, environment ...string) (*Process, error) {
	return StartCommand(executable, nil, workDir, environment...)
}

var pruneSocketsOnce sync.Once

func pruneStaleSockets() {
	pruneSocketsOnce.Do(func() { removeStaleSockets(os.TempDir(), os.Getpid()) })
}

func removeStaleSockets(base string, self int) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, entry := range entries {
		pid, ok := socketPID(entry.Name())
		if !ok || pid == self || processAlive(pid) {
			continue
		}
		_ = os.Remove(filepath.Join(base, entry.Name()))
	}
}

// StartCommand starts a renderer command and waits until its socket answers.
func StartCommand(executable string, args []string, workDir string, environment ...string) (*Process, error) {
	if executable == "" {
		return nil, errors.New("empty renderer executable")
	}
	socket, err := rendererSocket()
	if err != nil {
		return nil, err
	}
	running, err := startChild(executable, args, workDir, append([]string{"BIFROST_SOCKET=" + socket}, environment...)...)
	if err != nil {
		return nil, err
	}
	process := &Process{child: running, socket: socket}
	if err := process.waitHealthy(); err != nil {
		_ = process.Close(context.Background())
		return nil, err
	}
	return process, nil
}

// Attach connects to an already-running renderer listening on socket. The
// caller owns the renderer process lifecycle; Attach never starts, restarts,
// or removes the socket.
func Attach(socket string) (*Process, error) {
	if socket == "" {
		return nil, errors.New("empty renderer socket")
	}
	process := &Process{socket: socket}
	if err := process.waitHealthy(); err != nil {
		return nil, err
	}
	return process, nil
}

// Recover drops pooled connections after the remote renderer restarted on the
// same socket, so the next render dials again.
func (p *Process) Recover() {
	if p == nil {
		return
	}
	p.mu.Lock()
	idle := p.idle
	p.idle = nil
	p.mu.Unlock()
	for _, conn := range idle {
		_ = conn.conn.Close()
	}
}

func (p *Process) waitHealthy() error {
	deadline := time.Now().Add(startupTimeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		err := p.Healthy(ctx)
		cancel()
		if err == nil {
			return nil
		}
		if p.child != nil && p.child.exited() {
			if err := p.processError(); err != nil {
				return fmt.Errorf("renderer exited during startup: %w", err)
			}
			return errors.New("renderer exited during startup")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("renderer did not start within %s", startupTimeout)
}

// Healthy pings the renderer on a fresh connection, so a readiness check never
// interleaves with a render.
func (p *Process) Healthy(ctx context.Context) error {
	if p == nil || p.socket == "" {
		return errors.New("renderer process is not initialized")
	}
	if p.child != nil && p.child.exited() {
		if err := p.processError(); err != nil {
			return fmt.Errorf("renderer exited: %w", err)
		}
		return errors.New("renderer exited")
	}
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", p.socket)
	if err != nil {
		return fmt.Errorf("renderer health request: %w", err)
	}
	defer func() { _ = conn.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if _, err := conn.Write(make([]byte, 4)); err != nil {
		return fmt.Errorf("renderer health request: %w", err)
	}
	var header [5]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return fmt.Errorf("renderer health response: %w", err)
	}
	if header[0] != frameDone {
		return fmt.Errorf("renderer health frame %d", header[0])
	}
	return nil
}

func rendererSocket() (string, error) {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("renderer socket ID: %w", err)
	}
	socket := filepath.Join(os.TempDir(), fmt.Sprintf("bifrost-%d-%s.sock", os.Getpid(), hex.EncodeToString(random[:])))
	pruneStaleSockets()
	_ = os.Remove(socket)
	return socket, nil
}

func encodeRenderRequest(entry string, props json.RawMessage) ([]byte, error) {
	quoted, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}
	if len(props) == 0 {
		props = emptyPayloadProps
	}
	body := make([]byte, 0, len(quoted)+len(props)+20)
	body = append(body, 0, 0, 0, 0)
	body = append(body, `{"entry":`...)
	body = append(body, quoted...)
	body = append(body, `,"props":`...)
	body = append(body, props...)
	body = append(body, '}')
	binary.BigEndian.PutUint32(body, uint32(len(body)-4))
	return body, nil
}

// Render sends the request and streams frames into sink. A transport failure
// drops the connection, so the next render dials again.
func (p *Process) Render(ctx context.Context, entry string, props json.RawMessage, sink Sink) (err error) {
	request, err := encodeRenderRequest(entry, props)
	if err != nil {
		return fmt.Errorf("encode render request: %w", err)
	}
	conn, err := p.acquire(ctx)
	if err != nil {
		return unavailableOrCanceled(ctx, fmt.Errorf("renderer request: %w", err))
	}
	healthy := false
	stop := context.AfterFunc(ctx, func() { _ = conn.conn.Close() })
	defer func() {
		if !stop() {
			healthy = false
		}
		if healthy {
			p.release(conn)
			return
		}
		_ = conn.conn.Close()
	}()
	if _, err := conn.conn.Write(request); err != nil {
		return unavailableOrCanceled(ctx, fmt.Errorf("renderer request: %w", err))
	}

	sawHead := false
	var payload []byte
	var header [5]byte
	for {
		if _, err := io.ReadFull(conn.reader, header[:]); err != nil {
			return unavailableOrCanceled(ctx, fmt.Errorf("read renderer frame header: %w", err))
		}
		size := int(binary.BigEndian.Uint32(header[1:]))
		if size > maxWireFrameBytes {
			return &UnavailableError{Err: fmt.Errorf("renderer frame exceeds %d bytes", maxWireFrameBytes)}
		}
		if cap(payload) < size {
			payload = make([]byte, size)
		} else {
			payload = payload[:size]
		}
		if _, err := io.ReadFull(conn.reader, payload); err != nil {
			return unavailableOrCanceled(ctx, fmt.Errorf("read renderer frame payload: %w", err))
		}
		switch header[0] {
		case frameHead:
			if sawHead {
				return errors.New("renderer sent duplicate head frame")
			}
			sawHead = true
			if err := sink.Head(payload); err != nil {
				return err
			}
		case frameBody:
			if !sawHead {
				return errors.New("renderer sent body before head")
			}
			if err := sink.Body(payload); err != nil {
				return err
			}
		case frameDone:
			if size != 0 {
				return errors.New("renderer done frame has a payload")
			}
			if !sawHead {
				return errors.New("renderer completed without head")
			}
			healthy = true
			return nil
		case frameError:
			return &RemoteError{Message: string(payload)}
		default:
			return &UnavailableError{Err: fmt.Errorf("unknown renderer frame type %d", header[0])}
		}
	}
}

func (p *Process) acquire(ctx context.Context) (*pooledConn, error) {
	p.mu.Lock()
	if count := len(p.idle); count > 0 {
		conn := p.idle[count-1]
		p.idle = p.idle[:count-1]
		p.mu.Unlock()
		return conn, nil
	}
	p.mu.Unlock()
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", p.socket)
	if err != nil {
		return nil, err
	}
	return &pooledConn{conn: conn, reader: bufio.NewReaderSize(conn, 16<<10)}, nil
}

func (p *Process) release(conn *pooledConn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.idle) >= maxIdleConns {
		_ = conn.conn.Close()
		return
	}
	p.idle = append(p.idle, conn)
}

// Close drops the connection and stops the child.
func (p *Process) Close(ctx context.Context) error {
	if p == nil {
		return nil
	}
	p.Recover()
	p.child.stop(ctx)
	if p.cmd != nil && p.cmd.Process != nil {
		_ = os.Remove(p.socket)
	}
	return nil
}

func (c *child) processError() error {
	if c == nil {
		return nil
	}
	c.waitMu.Lock()
	defer c.waitMu.Unlock()
	return c.waitErr
}

func startChild(executable string, args []string, workDir string, environment ...string) (*child, error) {
	cmd := exec.Command(executable, args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), environment...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	configureParentDeath(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start renderer: %w", err)
	}
	running := &child{cmd: cmd, waitDone: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		running.waitMu.Lock()
		running.waitErr = err
		running.waitMu.Unlock()
		close(running.waitDone)
	}()
	return running, nil
}

func (c *child) stop(ctx context.Context) {
	if c == nil || c.cmd == nil || c.cmd.Process == nil {
		return
	}
	c.closeOnce.Do(func() {
		_ = c.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-c.waitDone:
		case <-ctx.Done():
			_ = c.cmd.Process.Kill()
			<-c.waitDone
		}
	})
}

func unavailableOrCanceled(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return &UnavailableError{Err: err}
}

func (c *child) exited() bool {
	select {
	case <-c.waitDone:
		return true
	default:
		return false
	}
}

func socketPID(name string) (int, bool) {
	if !strings.HasPrefix(name, "bifrost-") || !strings.HasSuffix(name, ".sock") {
		return 0, false
	}
	pid, _, found := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(name, "bifrost-"), ".sock"), "-")
	if !found {
		return 0, false
	}
	value, err := strconv.Atoi(pid)
	if err != nil {
		return 0, false
	}
	return value, true
}

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
