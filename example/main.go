package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"

	"github.com/3-lines-studio/bifrost"
	"github.com/go-chi/chi/v5"
)

//go:embed all:.bifrost
var bifrostFS embed.FS

func main() {
	r, err := bifrost.New(bifrost.WithAssetsFS(bifrostFS), bifrost.WithTiming())
	if err != nil {
		log.Fatalf("Failed to start renderer: %v", err)
	}
	defer r.Stop()

	homeHandler := r.NewPage("./pages/home.tsx", func(*http.Request) (map[string]interface{}, error) {
		return map[string]interface{}{
			"name": "World",
		}, nil
	})
	nestedHandler := r.NewPage("./pages/nested/page.tsx", func(*http.Request) (map[string]interface{}, error) {
		return map[string]interface{}{
			"name": "Nested",
		}, nil
	})
	aboutHandler := r.NewPage("./pages/about.tsx")
	messageHandler := r.NewPage("./pages/home.tsx", func(req *http.Request) (map[string]interface{}, error) {
		message := chi.URLParam(req, "message")
		if message == "" {
			message = "World"
		}
		return map[string]interface{}{
			"name": message,
		}, nil
	})
	errorHandler := r.NewPage("./pages/home.tsx", func(req *http.Request) (map[string]interface{}, error) {
		return nil, fmt.Errorf("this is a test error to verify the error page works correctly")
	})
	renderErrorHandler := r.NewPage("./pages/error-render.tsx")
	importErrorHandler := r.NewPage("./pages/error-import.tsx")

	router := chi.NewRouter()

	router.Handle("/", homeHandler)
	router.Handle("/about", aboutHandler)
	router.Handle("/nested", nestedHandler)
	router.Handle("/message/{message}", messageHandler)
	router.Handle("/error", errorHandler)
	router.Handle("/error-render", renderErrorHandler)
	router.Handle("/error-import", importErrorHandler)

	assetRouter := chi.NewRouter()
	bifrost.RegisterAssetRoutes(assetRouter, r, router)

	addr := ":8080"
	log.Printf("Serving on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, assetRouter); err != nil {
		log.Fatal(err)
	}
}
