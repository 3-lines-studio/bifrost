//go:build !windows

package renderproc

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

// Bridge is the Vite development server. Its render endpoint speaks HTTP so the
// same Unix socket can also carry Vite and HMR traffic.
type Bridge struct {
	*child
	socket string
	client *http.Client
}

func StartBridge(executable string, args []string, workDir string, environment ...string) (*Bridge, error) {
	socket, err := rendererSocket()
	if err != nil {
		return nil, err
	}
	running, err := startChild(executable, args, workDir, append([]string{"BIFROST_SOCKET=" + socket}, environment...)...)
	if err != nil {
		return nil, err
	}
	bridge := &Bridge{child: running, socket: socket, client: bridgeClient(socket)}
	if err := bridge.waitHealthy(); err != nil {
		_ = bridge.Close(context.Background())
		return nil, err
	}
	return bridge, nil
}

func AttachBridge(socket string) (*Bridge, error) {
	if socket == "" {
		return nil, errors.New("empty bridge socket")
	}
	bridge := &Bridge{socket: socket, client: bridgeClient(socket)}
	if err := bridge.waitHealthy(); err != nil {
		return nil, err
	}
	return bridge, nil
}

func bridgeClient(socket string) *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socket)
		},
		MaxIdleConns:        32,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	return &http.Client{Transport: transport}
}

func (b *Bridge) waitHealthy() error {
	deadline := time.Now().Add(startupTimeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		err := b.Healthy(ctx)
		cancel()
		if err == nil {
			return nil
		}
		if b.child != nil && b.child.exited() {
			if err := b.processError(); err != nil {
				return fmt.Errorf("bridge exited during startup: %w", err)
			}
			return errors.New("bridge exited during startup")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("bridge did not start within %s", startupTimeout)
}

func (b *Bridge) Healthy(ctx context.Context) error {
	if b == nil || b.client == nil {
		return errors.New("bridge is not initialized")
	}
	if b.child != nil && b.child.exited() {
		if err := b.processError(); err != nil {
			return fmt.Errorf("bridge exited: %w", err)
		}
		return errors.New("bridge exited")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://bifrost/health", nil)
	if err != nil {
		return err
	}
	response, err := b.client.Do(request)
	if err != nil {
		return fmt.Errorf("bridge health request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("bridge health status %d", response.StatusCode)
	}
	return nil
}

func (b *Bridge) Recover() {
	if b == nil || b.client == nil {
		return
	}
	if transport, ok := b.client.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
}

func (b *Bridge) Render(ctx context.Context, entry string, props json.RawMessage, sink Sink) error {
	request, err := encodeRenderRequest(entry, props)
	if err != nil {
		return fmt.Errorf("encode render request: %w", err)
	}
	message, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://bifrost/render", bytes.NewReader(request[4:]))
	if err != nil {
		return err
	}
	message.Header.Set("Content-Type", "application/json")
	response, err := b.client.Do(message)
	if err != nil {
		return bridgeUnavailableOrCanceled(ctx, fmt.Errorf("renderer request: %w", err))
	}
	defer func() { _ = response.Body.Close() }()

	sawHead := false
	var payload []byte
	var header [5]byte
	for {
		if _, err := io.ReadFull(response.Body, header[:]); err != nil {
			return bridgeUnavailableOrCanceled(ctx, fmt.Errorf("read renderer frame header: %w", err))
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
			return bridgeUnavailableOrCanceled(ctx, fmt.Errorf("read renderer frame payload: %w", err))
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
			return nil
		case frameError:
			return &RemoteError{Message: string(payload)}
		default:
			return &UnavailableError{Err: fmt.Errorf("unknown renderer frame type %d", header[0])}
		}
	}
}

func (b *Bridge) Close(ctx context.Context) error {
	if b == nil {
		return nil
	}
	b.Recover()
	b.child.stop(ctx)
	if b.child != nil && b.cmd != nil && b.cmd.Process != nil {
		_ = os.Remove(b.socket)
	}
	return nil
}

func bridgeUnavailableOrCanceled(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return &UnavailableError{Err: err}
}
