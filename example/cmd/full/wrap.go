// This file demonstrates using bifrost.Wrap() with chi router
// Run with: go run ./cmd/full/wrap.go

//go:build ignore

package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/3-lines-studio/bifrost"
	"github.com/3-lines-studio/bifrost/example"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	// Bifrost routes
	bifrostRoutes := []bifrost.Route{
		bifrost.Page("/{$}", "./pages/home.tsx", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{"name": "Chi Integration"}, nil
		})),
		bifrost.Page("/about", "./pages/about.tsx", bifrost.WithClient()),
		bifrost.Page("/product", "./pages/product.tsx", bifrost.WithStatic()),
	}

	app := bifrost.New(example.BifrostFS, bifrostRoutes...)
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

	log.Fatal(http.ListenAndServe(":8080", handler))
}
