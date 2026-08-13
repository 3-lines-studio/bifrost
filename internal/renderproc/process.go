//go:build !windows

package renderproc

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const startupTimeout = 10 * time.Second

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

// Process is one long-lived renderer executable.
type Process struct {
	cmd       *exec.Cmd
	socket    string
	client    *http.Client
	waitDone  chan struct{}
	waitMu    sync.Mutex
	waitErr   error
	closeOnce sync.Once
}

func Start(executable, workDir string, environment ...string) (*Process, error) {
	return StartCommand(executable, nil, workDir, environment...)
}

// StartCommand starts a renderer command and waits for its Unix socket health
// endpoint.
func StartCommand(executable string, args []string, workDir string, environment ...string) (*Process, error) {
	if executable == "" {
		return nil, errors.New("empty renderer executable")
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, fmt.Errorf("renderer socket ID: %w", err)
	}
	socket := filepath.Join(os.TempDir(), fmt.Sprintf("bifrost-%d-%s.sock", os.Getpid(), hex.EncodeToString(random[:])))
	_ = os.Remove(socket)

	cmd := exec.Command(executable, args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), append([]string{"BIFROST_SOCKET=" + socket}, environment...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	configureParentDeath(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start renderer: %w", err)
	}
	waitDone := make(chan struct{})

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socket)
		},
		MaxIdleConns:        32,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	process := &Process{
		cmd:      cmd,
		socket:   socket,
		client:   &http.Client{Transport: transport},
		waitDone: waitDone,
	}
	go func() {
		err := cmd.Wait()
		process.waitMu.Lock()
		process.waitErr = err
		process.waitMu.Unlock()
		close(waitDone)
	}()
	if err := process.waitHealthy(); err != nil {
		_ = process.Close(context.Background())
		return nil, err
	}
	return process, nil
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
		select {
		case <-p.waitDone:
			if err := p.processError(); err != nil {
				return fmt.Errorf("renderer exited during startup: %w", err)
			}
			return errors.New("renderer exited during startup")
		default:
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("renderer did not start within %s", startupTimeout)
}

// Healthy checks the child process and its health endpoint.
func (p *Process) Healthy(ctx context.Context) error {
	if p == nil || p.client == nil {
		return errors.New("renderer process is not initialized")
	}
	if p.waitDone != nil {
		select {
		case <-p.waitDone:
			if err := p.processError(); err != nil {
				return fmt.Errorf("renderer exited: %w", err)
			}
			return errors.New("renderer exited")
		default:
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://bifrost/health", nil)
	if err != nil {
		return err
	}
	response, err := p.client.Do(request)
	if err != nil {
		return fmt.Errorf("renderer health request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("renderer health status %d", response.StatusCode)
	}
	return nil
}

type renderPayload struct {
	Entry string          `json:"entry"`
	Props json.RawMessage `json:"props"`
}

const maxWireFrameBytes = 16 << 20

func (p *Process) Render(ctx context.Context, entry string, props json.RawMessage, sink Sink) error {
	body, err := json.Marshal(renderPayload{Entry: entry, Props: props})
	if err != nil {
		return fmt.Errorf("encode render request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://bifrost/render", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return &UnavailableError{Err: fmt.Errorf("renderer request: %w", err)}
	}
	defer func() { _ = response.Body.Close() }()

	sawHead := false
	var payload []byte
	var header [5]byte
	for {
		if _, err := io.ReadFull(response.Body, header[:]); err != nil {
			return &UnavailableError{Err: fmt.Errorf("read renderer frame header: %w", err)}
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
		if _, err := io.ReadFull(response.Body, payload); err != nil {
			return &UnavailableError{Err: fmt.Errorf("read renderer frame payload: %w", err)}
		}
		switch header[0] {
		case 1:
			if sawHead {
				return errors.New("renderer sent duplicate head frame")
			}
			sawHead = true
			if err := sink.Head(payload); err != nil {
				return err
			}
		case 2:
			if !sawHead {
				return errors.New("renderer sent body before head")
			}
			if err := sink.Body(payload); err != nil {
				return err
			}
		case 3:
			if size != 0 {
				return errors.New("renderer done frame has a payload")
			}
			if !sawHead {
				return errors.New("renderer completed without head")
			}
			return nil
		case 4:
			return &RemoteError{Message: string(payload)}
		default:
			return &UnavailableError{Err: fmt.Errorf("unknown renderer frame type %d", header[0])}
		}
	}
}

func (p *Process) processError() error {
	p.waitMu.Lock()
	defer p.waitMu.Unlock()
	return p.waitErr
}

func (p *Process) Close(ctx context.Context) error {
	var closeErr error
	p.closeOnce.Do(func() {
		if transport, ok := p.client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Signal(syscall.SIGTERM)
			select {
			case <-p.waitDone:
				err := p.processError()
				if err != nil {
					var exitErr *exec.ExitError
					if !errors.As(err, &exitErr) || exitErr.ExitCode() != -1 {
						closeErr = err
					}
				}
			case <-ctx.Done():
				_ = p.cmd.Process.Kill()
				<-p.waitDone
				closeErr = ctx.Err()
			}
		}
		_ = os.Remove(p.socket)
	})
	return closeErr
}
