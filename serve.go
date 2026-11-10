package bifrost

import (
	"context"
	"errors"
	"flag"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

func Serve(ctx context.Context, handler http.Handler) error {
	addr := os.Getenv("BIFROST_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	flag.StringVar(&addr, "addr", addr, "HTTP listen address")
	flag.Parse()

	listener := inheritedListener()
	server := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	startIdleShutdown(ctx, server)
	done := make(chan error, 1)
	if listener != nil {
		go func() { done <- server.Serve(listener) }()
	} else {
		go func() { done <- server.ListenAndServe() }()
	}

	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return err
	}
	err := <-done
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func inheritedListener() net.Listener {
	if os.Getenv("LISTEN_FDS") != "1" {
		return nil
	}
	if pid := os.Getenv("LISTEN_PID"); pid != "" && pid != strconv.Itoa(os.Getpid()) {
		return nil
	}
	file := os.NewFile(3, "bifrost-listener")
	if file == nil {
		return nil
	}
	listener, err := net.FileListener(file)
	_ = file.Close()
	if err != nil {
		return nil
	}
	return listener
}

func startIdleShutdown(ctx context.Context, server *http.Server) {
	value := os.Getenv("BIFROST_IDLE_TIMEOUT")
	if value == "" {
		return
	}
	idle, err := time.ParseDuration(value)
	if err != nil || idle <= 0 {
		return
	}
	var active atomic.Int64
	var connected sync.Map
	activity := make(chan struct{}, 1)
	notify := func() {
		select {
		case activity <- struct{}{}:
		default:
		}
	}
	server.ConnState = func(conn net.Conn, state http.ConnState) {
		switch state {
		case http.StateNew, http.StateActive:
			if _, loaded := connected.LoadOrStore(conn, struct{}{}); !loaded {
				active.Add(1)
			}
		case http.StateIdle, http.StateClosed, http.StateHijacked:
			if _, loaded := connected.LoadAndDelete(conn); loaded {
				active.Add(-1)
			}
		}
		notify()
	}
	go func() {
		timer := time.NewTimer(idle)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-activity:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(idle)
			case <-timer.C:
				if active.Load() == 0 {
					shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					_ = server.Shutdown(shutdownCtx)
					cancel()
					return
				}
				timer.Reset(idle)
			}
		}
	}()
}
