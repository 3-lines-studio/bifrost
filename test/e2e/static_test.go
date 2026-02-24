package e2e

import (
	"context"
	"testing"

	"github.com/3-lines-studio/bifrost"
)

func TestStaticProductPage_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/product", "./pages/product.tsx", bifrost.WithStatic()),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/product")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "static_product_dev", html)
}

func TestStaticProductPage_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/product", "./pages/product.tsx", bifrost.WithStatic()),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/product")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "static_product_prod", html)
}

func TestStaticBlogPage_Dev(t *testing.T) {
	skipIfNoBun(t)

	blogPosts := []struct {
		Slug  string
		Title string
		Body  string
	}{
		{Slug: "hello-world", Title: "Hello World", Body: "First post content"},
	}

	routes := []bifrost.Route{
		bifrost.Page("/blog/{slug...}", "./pages/blog.tsx",
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
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/blog/hello-world")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "static_blog_dev", html)
}

func TestStaticBlogPage_Prod(t *testing.T) {
	skipIfNoBun(t)

	// In production mode, static pages are served from pre-built HTML files
	// The data comes from main.go's build, not from the test's inline routes
	routes := []bifrost.Route{
		bifrost.Page("/blog/{slug...}", "./pages/blog.tsx",
			bifrost.WithStaticData(func(ctx context.Context) ([]bifrost.StaticPathData, error) {
				return []bifrost.StaticPathData{
					{
						Path: "/blog/getting-started",
						Props: map[string]any{
							"slug":  "getting-started",
							"title": "Getting Started",
							"body":  "How to start",
						},
					},
				}, nil
			})),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/blog/getting-started")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "static_blog_prod", html)
}

func TestStaticMultipleRoutes_Dev(t *testing.T) {
	skipIfNoBun(t)

	docPages := []struct {
		Path    string
		Title   string
		Content string
	}{
		{Path: "/docs/intro", Title: "Introduction", Content: "Welcome to the docs"},
		{Path: "/docs/api", Title: "API Reference", Content: "API documentation"},
	}

	routes := []bifrost.Route{
		bifrost.Page("/docs/{path...}", "./pages/blog.tsx",
			bifrost.WithStaticData(func(ctx context.Context) ([]bifrost.StaticPathData, error) {
				paths := make([]bifrost.StaticPathData, len(docPages))
				for i, doc := range docPages {
					paths[i] = bifrost.StaticPathData{
						Path: doc.Path,
						Props: map[string]any{
							"title": doc.Title,
							"body":  doc.Content,
						},
					}
				}
				return paths, nil
			})),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/docs/intro")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "static_docs_intro_dev", html)

	resp2, html2 := server.get(t, "/docs/api")
	assertHTTPStatus(t, resp2, 200)

	matchSnapshot(t, "static_docs_api_dev", html2)
}
