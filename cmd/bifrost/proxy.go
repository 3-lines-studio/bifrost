package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const wsPath = "/__bifrost_ws"
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

var reloadScript = []byte(`<script>new WebSocket('ws://'+location.host+'/__bifrost_ws').onmessage=()=>location.reload()</script>`)

type devProxy struct {
	port    int
	target  *url.URL
	server  *http.Server
	proxy   *httputil.ReverseProxy
	mu      sync.Mutex
	clients map[net.Conn]struct{}
}

func newDevProxy(port, appPort int) (*devProxy, error) {
	target, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", appPort))
	if err != nil {
		return nil, err
	}
	rp := httputil.NewSingleHostReverseProxy(target)
	dp := &devProxy{
		port:    port,
		target:  target,
		proxy:   rp,
		clients: make(map[net.Conn]struct{}),
	}
	rp.ModifyResponse = dp.injectReloadScript
	return dp, nil
}

func (p *devProxy) Start(ctx context.Context) (<-chan error, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", p.port))
	if err != nil {
		return nil, fmt.Errorf("port %d already in use: %w", p.port, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(wsPath, p.handleWebSocket)
	mux.Handle("/", p.proxy)

	p.server = &http.Server{Handler: mux}

	errCh := make(chan error, 1)
	go func() {
		errCh <- p.server.Serve(listener)
	}()
	return errCh, nil
}

func (p *devProxy) Stop(ctx context.Context) error {
	p.mu.Lock()
	for c := range p.clients {
		_ = c.Close()
		delete(p.clients, c)
	}
	p.mu.Unlock()
	if p.server == nil {
		return nil
	}
	return p.server.Shutdown(ctx)
}

func (p *devProxy) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !containsToken(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if r.Header.Get("Sec-WebSocket-Key") == "" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket not supported", http.StatusInternalServerError)
		return
	}

	conn, bufrw, err := hj.Hijack()
	if err != nil {
		return
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	h := sha1.New()
	h.Write([]byte(key + wsGUID))
	accept := base64.StdEncoding.EncodeToString(h.Sum(nil))

	_, _ = fmt.Fprintf(bufrw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)
	_ = bufrw.Flush()

	p.mu.Lock()
	p.clients[conn] = struct{}{}
	p.mu.Unlock()

	buf := make([]byte, 256)
	for {
		if _, err := bufrw.Read(buf); err != nil {
			break
		}
	}

	p.mu.Lock()
	delete(p.clients, conn)
	p.mu.Unlock()
	_ = conn.Close()
}

func (p *devProxy) BroadcastReload() {
	frame := []byte{0x81, 0x06, 'r', 'e', 'l', 'o', 'a', 'd'}
	p.mu.Lock()
	conns := make([]net.Conn, 0, len(p.clients))
	for conn := range p.clients {
		conns = append(conns, conn)
	}
	p.mu.Unlock()
	for _, conn := range conns {
		_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
		if _, err := conn.Write(frame); err != nil {
			p.mu.Lock()
			delete(p.clients, conn)
			p.mu.Unlock()
			_ = conn.Close()
		}
	}
}

func (p *devProxy) injectReloadScript(resp *http.Response) error {
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		return nil
	}
	if resp.Header.Get("Content-Encoding") != "" {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()

	lowered := bytes.ToLower(body)
	idx := bytes.LastIndex(lowered, []byte("</body>"))

	var newBody []byte
	if idx >= 0 {
		newBody = make([]byte, 0, len(body)+len(reloadScript))
		newBody = append(newBody, body[:idx]...)
		newBody = append(newBody, reloadScript...)
		newBody = append(newBody, body[idx:]...)
	} else {
		newBody = append(body, reloadScript...)
	}

	resp.Body = io.NopCloser(bytes.NewReader(newBody))
	resp.ContentLength = int64(len(newBody))
	resp.Header.Set("Content-Length", strconv.Itoa(len(newBody)))
	return nil
}

func containsToken(value, token string) bool {
	for part := range strings.SplitSeq(value, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

func waitForApp(ctx context.Context, appPort int, timeout time.Duration) error {
	addr := fmt.Sprintf("127.0.0.1:%d", appPort)
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if !time.Now().Before(deadline) {
				return fmt.Errorf("app on :%d did not start within %v", appPort, timeout)
			}
			conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				return nil
			}
		}
	}
}
