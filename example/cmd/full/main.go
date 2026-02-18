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
	app := bifrost.New(
		example.BifrostFS,
		bifrost.Page("/{$}", "./pages/home.tsx", bifrost.WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{
				"name": "World",
			}, nil
		})),
		bifrost.Page("/about", "./pages/about.tsx", bifrost.WithClient()),
		bifrost.Page("/nested", "./pages/nested/page.tsx", bifrost.WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{
				"name": "Nested",
			}, nil
		})),
		bifrost.Page("/blog/{slug...}", "./pages/blog.tsx",
			bifrost.WithStaticData(func(ctx context.Context) ([]bifrost.StaticPathData, error) {
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
			})),
		bifrost.Page("/message/{message}", "./pages/home.tsx", bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
			message := chi.URLParam(req, "message")
			if message == "" {
				message = "World"
			}
			return map[string]any{
				"name": message,
			}, nil
		})),
		bifrost.Page("/error", "./pages/home.tsx", bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
			return nil, fmt.Errorf("this is a test error to verify the error page works correctly")
		})),
		bifrost.Page("/error-render", "./pages/error-render.tsx"),
		bifrost.Page("/error-import", "./pages/error-import.tsx"),
	)
	defer app.Stop()

	log.Fatal(http.ListenAndServe(":8080", app.Handler()))
}
