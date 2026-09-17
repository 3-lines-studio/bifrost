package demo

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"
)

func Serve(ctx context.Context, handler http.Handler) error {
	addr := os.Getenv("BIFROST_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /old-docs/{rest...}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/"+r.PathValue("rest"), http.StatusMovedPermanently)
	})
	mux.Handle("/", handler)
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
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
	return server.Shutdown(shutdownCtx)
}
