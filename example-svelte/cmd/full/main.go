package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/3-lines-studio/bifrost"
	"github.com/3-lines-studio/bifrost/example-svelte"
)

// AuthRequiredError implements bifrost.RedirectError for authentication redirects
type AuthRequiredError struct{}

func (e *AuthRequiredError) Error() string {
	return "authentication required"
}

func (e *AuthRequiredError) RedirectURL() string {
	return "/login"
}

func (e *AuthRequiredError) RedirectStatusCode() int {
	return http.StatusFound
}

// AdminRequiredError implements bifrost.RedirectError with different redirect
type AdminRequiredError struct{}

func (e *AdminRequiredError) Error() string {
	return "admin access required"
}

func (e *AdminRequiredError) RedirectURL() string {
	return "/unauthorized"
}

func (e *AdminRequiredError) RedirectStatusCode() int {
	return http.StatusTemporaryRedirect
}

// BlogPost represents a blog entry for static generation
type BlogPost struct {
	Slug  string
	Title string
	Body  string
}

var blogPosts = []BlogPost{
	{Slug: "hello-world", Title: "Hello World", Body: "First post"},
	{Slug: "getting-started", Title: "Getting Started", Body: "How to start"},
}

func main() {
	routes := []bifrost.Route{
		// SSR Pages
		bifrost.Page("/{$}", "./pages/home.svelte", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{"name": "World"}, nil
		})),
		bifrost.Page("/simple", "./pages/home.svelte"),
		bifrost.Page("/user/{id}", "./pages/home.svelte", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{"name": r.PathValue("id")}, nil
		})),
		bifrost.Page("/search", "./pages/home.svelte", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			query := r.URL.Query().Get("q")
			if query == "" {
				query = "empty"
			}
			return map[string]any{"name": query}, nil
		})),

		// SSR - Shared component at multiple routes
		bifrost.Page("/shared-a", "./pages/home.svelte", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{"name": "Route A"}, nil
		})),
		bifrost.Page("/shared-b", "./pages/home.svelte", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{"name": "Route B"}, nil
		})),

		// SSR - Empty and nil loaders
		bifrost.Page("/empty", "./pages/home.svelte", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{}, nil
		})),

		// SSR - Pages needed for e2e tests
		bifrost.Page("/nested", "./pages/nested/page.svelte", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{"name": "Nested"}, nil
		})),
		bifrost.Page("/api-demo", "./pages/api-demo.svelte", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{
				"users": []map[string]any{
					{"id": 1, "name": "Alice", "email": "alice@example.com"},
					{"id": 2, "name": "Bob", "email": "bob@example.com"},
				},
				"loadTime": time.Now().Format("2006-01-02 15:04:05"),
			}, nil
		})),
		bifrost.Page("/message/{message}", "./pages/home.svelte", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			path := r.URL.Path
			message := "World"
			if len(path) > 9 && path[:9] == "/message/" {
				message = path[9:]
			}
			return map[string]any{"name": message}, nil
		})),

		// Client-Only Pages
		bifrost.Page("/client", "./pages/about.svelte", bifrost.WithClient()),
		bifrost.Page("/client/deep", "./pages/login.svelte", bifrost.WithClient()),
		bifrost.Page("/about", "./pages/about.svelte", bifrost.WithClient()),
		bifrost.Page("/login", "./pages/login.svelte", bifrost.WithClient()),

		// Static Pages
		bifrost.Page("/product", "./pages/product.svelte", bifrost.WithStatic()),
		bifrost.Page("/blog/{slug...}", "./pages/blog.svelte",
			bifrost.WithStaticData(func(ctx context.Context) ([]bifrost.StaticPathData, error) {
				paths := make([]bifrost.StaticPathData, len(blogPosts))
				for i, post := range blogPosts {
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

		// Dashboard with redirect demo
		bifrost.Page("/dashboard", "./pages/dashboard.svelte", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			if r.URL.Query().Get("demo") == "true" {
				return map[string]any{
					"user": map[string]string{
						"name": "Demo User",
						"role": "Administrator",
					},
				}, nil
			}
			return nil, &AuthRequiredError{}
		})),

		// Error Scenarios
		bifrost.Page("/error", "./pages/home.svelte", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return nil, fmt.Errorf("this is a test error to verify the error page works correctly")
		})),
		bifrost.Page("/error-loader", "./pages/home.svelte", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return nil, fmt.Errorf("loader failed")
		})),
		bifrost.Page("/error-redirect-302", "./pages/dashboard.svelte", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return nil, &AuthRequiredError{}
		})),
		bifrost.Page("/error-redirect-307", "./pages/dashboard.svelte", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return nil, &AdminRequiredError{}
		})),
		bifrost.Page("/error-render", "./pages/error-render.svelte"),
		bifrost.Page("/error-import", "./pages/error-import.svelte"),
	}

	app := bifrost.NewWithFramework(example.BifrostFS, bifrost.Svelte, routes...)
	defer app.Stop()

	fmt.Println("Server ready. Available routes:")
	fmt.Println("  SSR: /, /simple, /user/{id}, /search, /shared-a, /shared-b, /empty")
	fmt.Println("       /nested, /api-demo, /message/{id}, /dashboard")
	fmt.Println("  Client: /client, /client/deep, /about, /login")
	fmt.Println("  Static: /product, /blog/{slug}")
	fmt.Println("  Errors: /error, /error-loader, /error-redirect-302, /error-redirect-307, /error-render, /error-import")
	fmt.Println("")
	fmt.Println("This is the Svelte version of the example!")

	log.Fatal(http.ListenAndServe(":8080", app.Handler()))
}
