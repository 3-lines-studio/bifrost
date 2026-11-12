//go:build !windows

package renderproc

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type testSink struct {
	head string
	body string
}

func (s *testSink) Head(value []byte) error { s.head = string(value); return nil }
func (s *testSink) Body(value []byte) error { s.body += string(value); return nil }

type testRender struct {
	conn    net.Conn
	payload []byte
}

func rendererAt(t *testing.T) (*atomic.Int64, chan testRender, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "bifrost-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "render.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	accepted := &atomic.Int64{}
	requests := make(chan testRender, 16)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			go func() {
				defer conn.Close()
				for {
					var header [4]byte
					if _, err := io.ReadFull(conn, header[:]); err != nil {
						return
					}
					payload := make([]byte, binary.BigEndian.Uint32(header[:]))
					if _, err := io.ReadFull(conn, payload); err != nil {
						return
					}
					requests <- testRender{conn: conn, payload: payload}
				}
			}()
		}
	}()
	return accepted, requests, socket
}

func writeFrame(t *testing.T, conn net.Conn, kind byte, payload string) {
	t.Helper()
	var header [5]byte
	header[0] = kind
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	if _, err := conn.Write(header[:]); err != nil {
		t.Error(err)
	}
	if _, err := conn.Write([]byte(payload)); err != nil {
		t.Error(err)
	}
}

func TestRenderDecodesOrderedFrames(t *testing.T) {
	_, requests, socket := rendererAt(t)
	go func() {
		request := <-requests
		var payload struct {
			Entry string          `json:"entry"`
			Props json.RawMessage `json:"props"`
		}
		if err := json.Unmarshal(request.payload, &payload); err != nil {
			t.Error(err)
		}
		if payload.Entry != "entry.js" || string(payload.Props) != `{"name":"Don"}` {
			t.Errorf("payload = %+v", payload)
		}
		writeFrame(t, request.conn, frameHead, "<title>Hi</title>")
		writeFrame(t, request.conn, frameBody, "<main>")
		writeFrame(t, request.conn, frameBody, "Hi</main>")
		writeFrame(t, request.conn, frameDone, "")
	}()
	process := &Process{socket: socket}
	sink := &testSink{}
	if err := process.Render(context.Background(), "entry.js", json.RawMessage(`{"name":"Don"}`), sink); err != nil {
		t.Fatal(err)
	}
	if sink.head != "<title>Hi</title>" || sink.body != "<main>Hi</main>" {
		t.Fatalf("sink = %+v", sink)
	}
}

func TestRenderReusesOneConnection(t *testing.T) {
	accepted, requests, socket := rendererAt(t)
	go func() {
		for range 2 {
			request := <-requests
			writeFrame(t, request.conn, frameHead, "<title>Hi</title>")
			writeFrame(t, request.conn, frameDone, "")
		}
	}()
	process := &Process{socket: socket}
	for range 2 {
		if err := process.Render(context.Background(), "entry.js", nil, &testSink{}); err != nil {
			t.Fatal(err)
		}
	}
	if got := accepted.Load(); got != 1 {
		t.Fatalf("connections = %d, want 1", got)
	}
}

func TestHealthyPingsOnItsOwnConnection(t *testing.T) {
	_, requests, socket := rendererAt(t)
	process := &Process{socket: socket}
	done := make(chan error, 1)
	go func() { done <- process.Healthy(context.Background()) }()
	request := <-requests
	if len(request.payload) != 0 {
		t.Errorf("health request payload = %s", request.payload)
	}
	writeFrame(t, request.conn, frameDone, "")
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestHealthyFailsWhenRendererLeaves(t *testing.T) {
	_, requests, socket := rendererAt(t)
	process := &Process{socket: socket}
	done := make(chan error, 1)
	go func() { done <- process.Healthy(context.Background()) }()
	request := <-requests
	_ = request.conn.Close()
	if err := <-done; err == nil {
		t.Fatal("unhealthy renderer reported ready")
	}
}

func TestRenderClassifiesRemoteError(t *testing.T) {
	_, requests, socket := rendererAt(t)
	go func() {
		request := <-requests
		writeFrame(t, request.conn, frameError, "bad component")
	}()
	process := &Process{socket: socket}
	err := process.Render(context.Background(), "entry.js", nil, &testSink{})
	var remote *RemoteError
	if !errors.As(err, &remote) || remote.Message != "bad component" {
		t.Fatalf("error = %T %v, want RemoteError", err, err)
	}
}

func TestRenderReturnsContextErrorWhenCanceledMidStream(t *testing.T) {
	_, requests, socket := rendererAt(t)
	started := make(chan struct{})
	go func() {
		request := <-requests
		writeFrame(t, request.conn, frameHead, "<title>Hi</title>")
		close(started)
		_, _ = io.Copy(io.Discard, request.conn)
	}()
	process := &Process{socket: socket}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-started; cancel() }()
	err := process.Render(ctx, "entry.js", nil, &testSink{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	var unavailable *UnavailableError
	if errors.As(err, &unavailable) {
		t.Fatalf("canceled render reported as unavailable: %v", err)
	}
}

func TestRenderReturnsContextErrorWhenCanceledBeforeRequest(t *testing.T) {
	_, _, socket := rendererAt(t)
	process := &Process{socket: socket}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := process.Render(ctx, "entry.js", nil, &testSink{})
	var unavailable *UnavailableError
	if !errors.Is(err, context.Canceled) || errors.As(err, &unavailable) {
		t.Fatalf("err = %v, want bare context.Canceled", err)
	}
}

func TestRenderStillReportsUnavailableWhenRendererDrops(t *testing.T) {
	_, requests, socket := rendererAt(t)
	go func() {
		request := <-requests
		writeFrame(t, request.conn, frameHead, "<title>Hi</title>")
		_ = request.conn.Close()
	}()
	process := &Process{socket: socket}
	err := process.Render(context.Background(), "entry.js", nil, &testSink{})
	var unavailable *UnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("err = %v, want UnavailableError", err)
	}
}

func TestRenderDialsAfterRecover(t *testing.T) {
	accepted, requests, socket := rendererAt(t)
	go func() {
		for range 2 {
			request := <-requests
			writeFrame(t, request.conn, frameHead, "<title>Hi</title>")
			writeFrame(t, request.conn, frameDone, "")
		}
	}()
	process := &Process{socket: socket}
	if err := process.Render(context.Background(), "entry.js", nil, &testSink{}); err != nil {
		t.Fatal(err)
	}
	process.Recover()
	if err := process.Render(context.Background(), "entry.js", nil, &testSink{}); err != nil {
		t.Fatal(err)
	}
	if got := accepted.Load(); got != 2 {
		t.Fatalf("connections = %d, want 2", got)
	}
}

func TestRuntimeSourcesPropagateCancellation(t *testing.T) {
	for _, required := range []string{
		"module.render(input.props ?? {}, controller.signal)",
		"reader.cancel(controller.signal.reason)",
		"state.abort?.abort()",
	} {
		if !strings.Contains(RuntimeSource, required) {
			t.Fatalf("production runtime does not contain %q", required)
		}
	}
	for _, required := range []string{
		"module.render(input.props ?? {}, request.signal)",
		"reader?.cancel(request.signal.reason)",
		"cancel(reason)",
	} {
		if !strings.Contains(DevRuntimeSource, required) {
			t.Fatalf("development runtime does not contain %q", required)
		}
	}
}

func TestRemoveStaleSocketsKeepsLiveOwners(t *testing.T) {
	base := t.TempDir()
	stale := filepath.Join(base, "bifrost-99999999-aaaaaaaaaaaaaaaa.sock")
	own := filepath.Join(base, "bifrost-12345-bbbbbbbbbbbbbbbb.sock")
	live := filepath.Join(base, fmt.Sprintf("bifrost-%d-cccccccccccccccc.sock", os.Getpid()))
	unrelated := filepath.Join(base, "bifrost-runtime-abc")
	for _, path := range []string{stale, own, live, unrelated} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	removeStaleSockets(base, 12345)
	if _, err := os.Stat(stale); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stale socket survived: %v", err)
	}
	for _, kept := range []string{own, live, unrelated} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("%s was removed: %v", kept, err)
		}
	}
}

func TestSocketPID(t *testing.T) {
	cases := map[string]int{
		"bifrost-4242-abcdef0123456789.sock": 4242,
		"bifrost-7-x.sock":                   7,
	}
	for name, want := range cases {
		got, ok := socketPID(name)
		if !ok || got != want {
			t.Fatalf("socketPID(%q) = %d, %v", name, got, ok)
		}
	}
	for _, name := range []string{"bifrost-runtime-abc", "bifrost-42.sock", "unrelated.sock", "bifrost-abc-def.sock"} {
		if _, ok := socketPID(name); ok {
			t.Fatalf("socketPID(%q) matched", name)
		}
	}
}

func TestEncodeRenderRequestLength(t *testing.T) {
	request, err := encodeRenderRequest("ssr/app.js", json.RawMessage(`{"title":"Hello \u003cworld\u003e"}`))
	if err != nil {
		t.Fatal(err)
	}
	if size := binary.BigEndian.Uint32(request); int(size) != len(request)-4 {
		t.Fatalf("length prefix = %d for %d bytes", size, len(request)-4)
	}
	want := `{"entry":"ssr/app.js","props":{"title":"Hello \u003cworld\u003e"}}`
	if string(request[4:]) != want {
		t.Fatalf("payload = %s", request[4:])
	}
	request, err = encodeRenderRequest(`ssr/a"b.js`, nil)
	if err != nil {
		t.Fatal(err)
	}
	want = `{"entry":"ssr/a\"b.js","props":{}}`
	if string(request[4:]) != want {
		t.Fatalf("payload = %s", request[4:])
	}
}

func TestRenderDeadlineDoesNotLeakGoroutines(t *testing.T) {
	_, requests, socket := rendererAt(t)
	go func() {
		request := <-requests
		writeFrame(t, request.conn, frameHead, "<title>Hi</title>")
		_, _ = io.Copy(io.Discard, request.conn)
	}()
	process := &Process{socket: socket}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := process.Render(ctx, "entry.js", nil, &testSink{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}
