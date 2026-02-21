package e2e

import (
	"net/http"
	"testing"

	"github.com/3-lines-studio/bifrost"
)

func TestWrapWithServeMux_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/{$}", "./pages/home.tsx", bifrost.WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{"name": "Wrapped"}, nil
		})),
		bifrost.Page("/about", "./pages/about.tsx", bifrost.WithClient()),
	}

	server := newTestServerWithWrap(t, routes, true)
	server.start(t)

	// Test Bifrost SSR page through wrapped handler
	resp, html := server.get(t, "/")
	assertHTTPStatus(t, resp, 200)
	matchSnapshot(t, "wrap_mux_page_dev", html)

	// Test Bifrost client page through wrapped handler
	resp2, html2 := server.get(t, "/about")
	assertHTTPStatus(t, resp2, 200)
	matchSnapshot(t, "wrap_mux_client_dev", html2)

	// Test that API route also works
	resp3, _ := server.get(t, "/api/health")
	assertHTTPStatus(t, resp3, 200)
}

func TestWrapWithServeMux_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/{$}", "./pages/home.tsx", bifrost.WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{"name": "Wrapped"}, nil
		})),
		bifrost.Page("/about", "./pages/about.tsx", bifrost.WithClient()),
	}

	server := newTestServerWithWrap(t, routes, false)
	server.start(t)

	// Test Bifrost SSR page through wrapped handler
	resp, html := server.get(t, "/")
	assertHTTPStatus(t, resp, 200)
	matchSnapshot(t, "wrap_mux_page_prod", html)

	// Test Bifrost client page through wrapped handler
	resp2, html2 := server.get(t, "/about")
	assertHTTPStatus(t, resp2, 200)
	matchSnapshot(t, "wrap_mux_client_prod", html2)

	// Test that API route also works
	resp3, _ := server.get(t, "/api/health")
	assertHTTPStatus(t, resp3, 200)
}
