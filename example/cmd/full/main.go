package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/3-lines-studio/bifrost"
	"github.com/3-lines-studio/bifrost/example"
	"github.com/go-chi/chi/v5"
)

// Mock database for demonstration
type Post struct {
	Slug  string
	Title string
	Body  string
}

var posts = []Post{
	{Slug: "hello-world", Title: "Hello World", Body: "This is the first post"},
	{Slug: "getting-started", Title: "Getting Started", Body: "How to get started with Bifrost"},
}

func main() {
	r, err := bifrost.New(bifrost.WithAssetsFS(example.BifrostFS))
	if err != nil {
		log.Fatalf("Failed to start renderer: %v", err)
	}
	defer r.Stop()

	homeHandler := r.NewPage("./pages/home.tsx", bifrost.WithPropsLoader(func(*http.Request) (map[string]any, error) {
		return map[string]any{
			"name": "World",
		}, nil
	}))
	nestedHandler := r.NewPage("./pages/nested/page.tsx", bifrost.WithPropsLoader(func(*http.Request) (map[string]any, error) {
		return map[string]any{
			"name": "Nested",
		}, nil
	}))
	aboutHandler := r.NewPage("./pages/about.tsx", bifrost.WithClientOnly())

	// Dynamic static pages - prerendered at build time with data from Go
	blogHandler := r.NewPage("./pages/blog.tsx",
		bifrost.WithStaticPrerender(),
		bifrost.WithStaticDataLoader(func(ctx context.Context) ([]bifrost.StaticPathData, error) {
			// In real usage, this would query your database or CMS
			paths := make([]bifrost.StaticPathData, len(posts))
			for i, post := range posts {
				paths[i] = bifrost.StaticPathData{
					Path: "/blog/" + post.Slug,
					Props: map[string]any{
						"slug":  post.Slug,
						"title": post.Title,
						"body":  post.Body,
					},
				}
			}
			return paths, nil
		}),
	)

	messageHandler := r.NewPage("./pages/home.tsx", bifrost.WithPropsLoader(func(req *http.Request) (map[string]any, error) {
		message := chi.URLParam(req, "message")
		if message == "" {
			message = "World"
		}
		return map[string]any{
			"name": message,
		}, nil
	}))
	errorHandler := r.NewPage("./pages/home.tsx", bifrost.WithPropsLoader(func(req *http.Request) (map[string]any, error) {
		return nil, fmt.Errorf("this is a test error to verify the error page works correctly")
	}))
	renderErrorHandler := r.NewPage("./pages/error-render.tsx")
	importErrorHandler := r.NewPage("./pages/error-import.tsx")

	router := chi.NewRouter()

	router.Handle("/", homeHandler)
	router.Handle("/about", aboutHandler)
	router.Handle("/nested", nestedHandler)
	router.Handle("/blog/*", blogHandler) // Dynamic static pages
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
