package e2e

import (
	"net/http"
	"testing"

	"github.com/3-lines-studio/bifrost"
)

// TestProd_RuntimeInitialized_SSR verifies that SSR apps initialize runtime in production
func TestProd_RuntimeInitialized_SSR(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/", "./pages/home.tsx", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{"name": "SSR App"}, nil
		})),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	// The app should start without panic and serve SSR content
	resp, html := server.get(t, "/")
	assertHTTPStatus(t, resp, 200)

	// Verify SSR content is rendered (uses runtime)
	if resp.StatusCode != 200 {
		t.Errorf("expected SSR page to work with runtime, got status %d", resp.StatusCode)
	}

	// Should be able to make multiple requests (runtime stays running)
	resp2, _ := server.get(t, "/")
	assertHTTPStatus(t, resp2, 200)

	matchSnapshot(t, "prod_runtime_initialized_ssr", html)
}

// TestProd_NoRuntime_StaticOnly verifies that static-only apps work without runtime
// Uses example app route: /product (Static)
// This is the CRITICAL test - the website pattern
func TestProd_NoRuntime_StaticOnly(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/product", "./pages/product.tsx", bifrost.WithStatic()),
	}

	// This should NOT panic due to missing runtime
	server := newTestServer(t, routes, false)
	server.start(t)

	// Static page should serve without runtime
	resp, html := server.get(t, "/product")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "prod_no_runtime_static_only", html)
}

// TestProd_NoRuntime_ClientOnly verifies that client-only apps work without runtime
func TestProd_NoRuntime_ClientOnly(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/login", "./pages/login.tsx", bifrost.WithClient()),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/login")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "prod_no_runtime_client_only", html)
}

// TestProd_RuntimeInitialized_MixedWithSSR verifies mixed apps with SSR use runtime
// Uses example app routes: /dashboard (SSR), /product (Static), /about (Client)
func TestProd_RuntimeInitialized_MixedWithSSR(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/dashboard", "./pages/dashboard.tsx", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{
				"user": map[string]string{"name": "Dashboard", "role": "Admin"},
			}, nil
		})),
		bifrost.Page("/product", "./pages/product.tsx", bifrost.WithStatic()),
		bifrost.Page("/about", "./pages/about.tsx", bifrost.WithClient()),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	// SSR page uses runtime
	resp1, html1 := server.get(t, "/dashboard?demo=true")
	assertHTTPStatus(t, resp1, 200)

	// Static page works without runtime for this request
	resp2, html2 := server.get(t, "/product")
	assertHTTPStatus(t, resp2, 200)

	// Client page works
	resp3, html3 := server.get(t, "/about")
	assertHTTPStatus(t, resp3, 200)

	matchSnapshot(t, "prod_runtime_mixed_dashboard", html1)
	matchSnapshot(t, "prod_runtime_mixed_product", html2)
	matchSnapshot(t, "prod_runtime_mixed_about", html3)
}

// TestProd_NoRuntime_MixedWithoutSSR verifies mixed apps without SSR work without runtime
// Uses example app routes: /product (Static), /about (Client)
func TestProd_NoRuntime_MixedWithoutSSR(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/product", "./pages/product.tsx", bifrost.WithStatic()),
		bifrost.Page("/about", "./pages/about.tsx", bifrost.WithClient()),
	}

	// This should NOT panic - no runtime needed
	server := newTestServer(t, routes, false)
	server.start(t)

	// Static page serves
	resp1, html1 := server.get(t, "/product")
	assertHTTPStatus(t, resp1, 200)

	// Client page serves
	resp2, html2 := server.get(t, "/about")
	assertHTTPStatus(t, resp2, 200)

	matchSnapshot(t, "prod_no_runtime_mixed_product", html1)
	matchSnapshot(t, "prod_no_runtime_mixed_about", html2)
}
