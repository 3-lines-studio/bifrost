package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

type devProxy struct {
	port   int
	target *url.URL
	server *http.Server
	proxy  *httputil.ReverseProxy
}

func newDevProxy(port, appPort int) (*devProxy, error) {
	target, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", appPort))
	if err != nil {
		return nil, err
	}
	return &devProxy{
		port:   port,
		target: target,
		proxy:  httputil.NewSingleHostReverseProxy(target),
	}, nil
}

func (p *devProxy) Start(ctx context.Context) (<-chan error, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", p.port))
	if err != nil {
		return nil, fmt.Errorf("port %d already in use: %w", p.port, err)
	}
	p.server = &http.Server{Handler: p.proxy}
	errCh := make(chan error, 1)
	go func() {
		errCh <- p.server.Serve(listener)
	}()
	return errCh, nil
}

func (p *devProxy) Stop(ctx context.Context) error {
	if p.server == nil {
		return nil
	}
	return p.server.Shutdown(ctx)
}

func waitForApp(ctx context.Context, appPort int, timeout time.Duration) error {
	addr := fmt.Sprintf("127.0.0.1:%d", appPort)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("app on :%d did not start within %v", appPort, timeout)
}
