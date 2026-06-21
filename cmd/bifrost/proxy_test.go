package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func getFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate free port: %v", err)
	}
	defer func() { _ = ln.Close() }()

	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("failed to parse port: %v", err)
	}
	return port
}

func TestNewDevProxy(t *testing.T) {
	p, err := newDevProxy(3000, 8080)
	if err != nil {
		t.Fatalf("newDevProxy error = %v", err)
	}
	if p == nil {
		t.Fatal("newDevProxy returned nil")
	}
	if p.port != 3000 {
		t.Errorf("port = %d, want 3000", p.port)
	}
	if p.target.String() != "http://127.0.0.1:8080" {
		t.Errorf("target = %q, want http://127.0.0.1:8080", p.target.String())
	}
}

func TestWaitForApp_OK(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("failed to parse port: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := waitForApp(ctx, port, 2*time.Second); err != nil {
		t.Fatalf("waitForApp error = %v", err)
	}
}

func TestWaitForApp_Timeout(t *testing.T) {
	// Use a port that is extremely unlikely to be listening.
	ctx := context.Background()
	err := waitForApp(ctx, 54321, 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		// waitForApp returns a custom error on timeout, which is fine.
		if err.Error() == "" {
			t.Fatal("expected non-empty timeout error")
		}
	}
}

func getPortFromURL(t *testing.T, url string) int {
	t.Helper()
	_, portStr, err := net.SplitHostPort(strings.TrimPrefix(url, "http://"))
	if err != nil {
		t.Fatalf("failed to parse URL %q: %v", url, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("failed to parse port: %v", err)
	}
	return port
}

func TestDevProxy_StartStop(t *testing.T) {
	port := getFreePort(t)
	p, err := newDevProxy(port, port+1)
	if err != nil {
		t.Fatalf("newDevProxy error = %v", err)
	}

	ctx := context.Background()
	errCh, err := p.Start(ctx)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}

	// Give the server a moment to start accepting connections.
	time.Sleep(50 * time.Millisecond)

	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.Stop(stopCtx); err != nil {
		t.Fatalf("Stop error = %v", err)
	}

	select {
	case serveErr := <-errCh:
		if serveErr != nil && serveErr != http.ErrServerClosed {
			t.Fatalf("Serve returned unexpected error: %v", serveErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal(" Serve did not return after Stop")
	}
}

func TestDevProxy_InjectsReloadScript(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, "<html><body>hi</body></html>")
	}))
	defer backend.Close()

	backendPort := getPortFromURL(t, backend.URL)
	port := getFreePort(t)
	p, err := newDevProxy(port, backendPort)
	if err != nil {
		t.Fatalf("newDevProxy error = %v", err)
	}

	ctx := context.Background()
	_, err = p.Start(ctx)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.Stop(stopCtx)
	}()

	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatalf("GET error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "new WebSocket") {
		t.Errorf("expected injected script, got:\n%s", bodyStr)
	}
	scriptIdx := strings.Index(bodyStr, "<script>")
	bodyIdx := strings.Index(bodyStr, "</body>")
	if scriptIdx < 0 || bodyIdx < 0 || scriptIdx > bodyIdx {
		t.Errorf("script should appear before </body>, got script at %d, </body> at %d", scriptIdx, bodyIdx)
	}
}

func TestDevProxy_NoInjectionForNonHTML(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, "hello")
	}))
	defer backend.Close()

	backendPort := getPortFromURL(t, backend.URL)
	port := getFreePort(t)
	p, err := newDevProxy(port, backendPort)
	if err != nil {
		t.Fatalf("newDevProxy error = %v", err)
	}

	ctx := context.Background()
	_, err = p.Start(ctx)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.Stop(stopCtx)
	}()

	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatalf("GET error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}
	if strings.Contains(string(body), "WebSocket") {
		t.Errorf("expected no injection for text/plain")
	}
}

func TestDevProxy_NoInjectionForGzipped(t *testing.T) {
	var gzipBuf bytes.Buffer
	gw := gzip.NewWriter(&gzipBuf)
	_, _ = gw.Write([]byte("<html><body>hi</body></html>"))
	_ = gw.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(gzipBuf.Bytes())
	}))
	defer backend.Close()

	backendPort := getPortFromURL(t, backend.URL)
	port := getFreePort(t)
	p, err := newDevProxy(port, backendPort)
	if err != nil {
		t.Fatalf("newDevProxy error = %v", err)
	}

	ctx := context.Background()
	_, err = p.Start(ctx)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.Stop(stopCtx)
	}()

	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatalf("GET error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}
	if strings.Contains(string(body), "WebSocket") {
		t.Errorf("expected no injection for gzipped content")
	}
}

func TestDevProxy_WebSocketHandshake(t *testing.T) {
	port := getFreePort(t)
	p, err := newDevProxy(port, port+1)
	if err != nil {
		t.Fatalf("newDevProxy error = %v", err)
	}

	ctx := context.Background()
	_, err = p.Start(ctx)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.Stop(stopCtx)
	}()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("dial error = %v", err)
	}
	defer func() { _ = conn.Close() }()

	req := "GET /__bifrost_ws HTTP/1.1\r\nHost: 127.0.0.1\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n"
	_, _ = conn.Write([]byte(req))

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read error = %v", err)
	}

	resp := string(buf[:n])
	if !strings.Contains(resp, "101 Switching Protocols") {
		t.Errorf("expected 101 response, got:\n%s", resp)
	}
	if !strings.Contains(resp, "Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=") {
		t.Errorf("expected correct Sec-WebSocket-Accept, got:\n%s", resp)
	}
}

func TestDevProxy_BroadcastReloadDeliversFrame(t *testing.T) {
	port := getFreePort(t)
	p, err := newDevProxy(port, port+1)
	if err != nil {
		t.Fatalf("newDevProxy error = %v", err)
	}

	ctx := context.Background()
	_, err = p.Start(ctx)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.Stop(stopCtx)
	}()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("dial error = %v", err)
	}
	defer func() { _ = conn.Close() }()

	req := "GET /__bifrost_ws HTTP/1.1\r\nHost: 127.0.0.1\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n"
	_, _ = conn.Write([]byte(req))

	handshakeBuf := make([]byte, 1024)
	_, err = conn.Read(handshakeBuf)
	if err != nil {
		t.Fatalf("handshake read error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	p.BroadcastReload()

	frame := make([]byte, 8)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(frame)
	if err != nil {
		t.Fatalf("frame read error = %v", err)
	}
	if n < 2 {
		t.Fatalf("expected at least 2 bytes, got %d", n)
	}
	if frame[0] != 0x81 {
		t.Errorf("expected opcode 0x81 (text frame), got 0x%02x", frame[0])
	}
	if frame[1] != 0x06 {
		t.Errorf("expected length 0x06, got 0x%02x", frame[1])
	}
	if n >= 8 && string(frame[2:8]) != "reload" {
		t.Errorf("expected payload 'reload', got %q", string(frame[2:8]))
	}
}

func TestDevProxy_WebSocketRejectsMissingKey(t *testing.T) {
	port := getFreePort(t)
	p, err := newDevProxy(port, port+1)
	if err != nil {
		t.Fatalf("newDevProxy error = %v", err)
	}

	ctx := context.Background()
	_, err = p.Start(ctx)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.Stop(stopCtx)
	}()

	time.Sleep(50 * time.Millisecond)

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/__bifrost_ws", port), nil)
	if err != nil {
		t.Fatalf("NewRequest error = %v", err)
	}
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Version", "13")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing Sec-WebSocket-Key, got %d", resp.StatusCode)
	}
}

func TestDevProxy_WebSocketRejectsBadVersion(t *testing.T) {
	port := getFreePort(t)
	p, err := newDevProxy(port, port+1)
	if err != nil {
		t.Fatalf("newDevProxy error = %v", err)
	}

	ctx := context.Background()
	_, err = p.Start(ctx)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.Stop(stopCtx)
	}()

	time.Sleep(50 * time.Millisecond)

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/__bifrost_ws", port), nil)
	if err != nil {
		t.Fatalf("NewRequest error = %v", err)
	}
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for unsupported Sec-WebSocket-Version, got %d", resp.StatusCode)
	}
}

func TestDevProxy_InjectReloadScriptNoBodyTag(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, "<html><head><title>hi</title></head></html>")
	}))
	defer backend.Close()

	backendPort := getPortFromURL(t, backend.URL)
	port := getFreePort(t)
	p, err := newDevProxy(port, backendPort)
	if err != nil {
		t.Fatalf("newDevProxy error = %v", err)
	}

	ctx := context.Background()
	_, err = p.Start(ctx)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.Stop(stopCtx)
	}()

	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatalf("GET error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "new WebSocket") {
		t.Errorf("expected injected script appended to HTML without </body>, got:\n%s", bodyStr)
	}
	if resp.Header.Get("Content-Length") != strconv.Itoa(len(body)) {
		t.Errorf("Content-Length header %q does not match actual body length %d", resp.Header.Get("Content-Length"), len(body))
	}
}

func TestDevProxy_InjectReloadScriptUppercaseBody(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, "<HTML><BODY>hi</BODY></HTML>")
	}))
	defer backend.Close()

	backendPort := getPortFromURL(t, backend.URL)
	port := getFreePort(t)
	p, err := newDevProxy(port, backendPort)
	if err != nil {
		t.Fatalf("newDevProxy error = %v", err)
	}

	ctx := context.Background()
	_, err = p.Start(ctx)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.Stop(stopCtx)
	}()

	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatalf("GET error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "new WebSocket") {
		t.Errorf("expected injected script for uppercase </BODY>, got:\n%s", bodyStr)
	}
	scriptIdx := strings.Index(bodyStr, "<script>")
	bodyIdx := strings.Index(strings.ToLower(bodyStr), "</body>")
	if scriptIdx < 0 || bodyIdx < 0 || scriptIdx > bodyIdx {
		t.Errorf("script should appear before </body>, got script at %d, </body> at %d", scriptIdx, bodyIdx)
	}
}

func TestDevProxy_BroadcastReloadConcurrentStop(t *testing.T) {
	port := getFreePort(t)
	p, err := newDevProxy(port, port+1)
	if err != nil {
		t.Fatalf("newDevProxy error = %v", err)
	}

	ctx := context.Background()
	_, err = p.Start(ctx)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Connect several clients.
	var conns []net.Conn
	for range 5 {
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			t.Fatalf("dial error = %v", err)
		}
		req := "GET /__bifrost_ws HTTP/1.1\r\nHost: 127.0.0.1\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n"
		_, _ = conn.Write([]byte(req))
		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)
		conns = append(conns, conn)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 20 {
			p.BroadcastReload()
		}
	}()
	go func() {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond)
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.Stop(stopCtx)
	}()
	wg.Wait()

	for _, c := range conns {
		_ = c.Close()
	}
}

func TestDevProxy_MultipleClientsReceiveReload(t *testing.T) {
	port := getFreePort(t)
	p, err := newDevProxy(port, port+1)
	if err != nil {
		t.Fatalf("newDevProxy error = %v", err)
	}

	ctx := context.Background()
	_, err = p.Start(ctx)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.Stop(stopCtx)
	}()

	time.Sleep(50 * time.Millisecond)

	req := "GET /__bifrost_ws HTTP/1.1\r\nHost: 127.0.0.1\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n"
	var conns []net.Conn
	for range 3 {
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			t.Fatalf("dial error = %v", err)
		}
		_, _ = conn.Write([]byte(req))
		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)
		conns = append(conns, conn)
		defer func() { _ = conn.Close() }()
	}

	time.Sleep(100 * time.Millisecond)
	p.BroadcastReload()

	for i, conn := range conns {
		frame := make([]byte, 8)
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := conn.Read(frame)
		if err != nil {
			t.Fatalf("client %d frame read error = %v", i, err)
		}
		if n != 8 || frame[0] != 0x81 || frame[1] != 0x06 || string(frame[2:8]) != "reload" {
			t.Fatalf("client %d expected reload frame, got %v", i, frame[:n])
		}
	}
}

func TestDevProxy_WebSocketNoUpgrade400(t *testing.T) {
	port := getFreePort(t)
	p, err := newDevProxy(port, port+1)
	if err != nil {
		t.Fatalf("newDevProxy error = %v", err)
	}

	ctx := context.Background()
	_, err = p.Start(ctx)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.Stop(stopCtx)
	}()

	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/__bifrost_ws", port))
	if err != nil {
		t.Fatalf("GET error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}
