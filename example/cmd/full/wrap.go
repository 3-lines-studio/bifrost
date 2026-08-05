// This file demonstrates using bifrost.Wrap() with chi router
// Run with: go run ./cmd/full/wrap.go

//go:build ignore

package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/3-lines-studio/bifrost"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// go run ./cmd/full/wrap.go compiles this file alone, so bifrostFS is declared here
// too (embed.go is not part of a single-file build).
//
//go:embed all:.bifrost
var bifrostFS embed.FS

func main() {
	// Bifrost routes
	bifrostRoutes := []bifrost.Route{
		bifrost.Page("/{$}", "./pages/home.tsx", bifrost.WithLoader(func(r *http.Request) (any, error) {
			return map[string]any{"name": "Chi Integration"}, nil
		})),
		bifrost.Page("/about", "./pages/about.tsx", bifrost.WithClient()),
		bifrost.Page("/product", "./pages/product.tsx", bifrost.WithStatic()),
	}

	app, err := bifrost.New(bifrostFS, bifrostRoutes...)
	if err != nil {
		log.Fatalf("create app: %v", err)
	}
	defer app.Stop()

	// Create chi router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Add API routes BEFORE wrapping with Bifrost
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	r.Get("/api/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version":"1.0.0","framework":"bifrost+chi"}`))
	})

	// Wrap chi router with Bifrost - Bifrost handles page routes
	handler := app.Wrap(r)

	fmt.Println("Server running with chi router integration")
	fmt.Println("  Bifrost pages: /, /about, /product")
	fmt.Println("  API routes: /api/health, /api/info")

	server := &http.Server{
		Addr:              ":8080",
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown: %v", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("server stopped: %v", err)
	}
}
