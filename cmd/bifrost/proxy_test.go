package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
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
