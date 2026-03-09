package e2e

import (
	"net/http"
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost"
)

// TestMixedMode_AllThree_Dev tests an app with SSR, Static, and Client pages in dev mode
func TestMixedMode_AllThree_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		// SSR page - needs runtime
		bifrost.Page("/dashboard", "./pages/home.tsx", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{"name": "Dashboard"}, nil
		})),
		// Static page - pre-rendered
		bifrost.Page("/about", "./pages/about.tsx", bifrost.WithStatic()),
		// Client-only page - empty shell
		bifrost.Page("/admin", "./pages/login.tsx", bifrost.WithClient()),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	// Test SSR page renders dynamically
	resp1, html1 := server.get(t, "/dashboard")
	assertHTTPStatus(t, resp1, 200)
	if !strings.Contains(html1, "Dashboard") {
		t.Error("expected SSR page to contain 'Dashboard'")
	}

	// Test static page serves
	resp2, html2 := server.get(t, "/about")
	assertHTTPStatus(t, resp2, 200)
	if !strings.Contains(html2, "About") {
		t.Error("expected static page to contain 'About'")
	}

	// Test client page serves empty shell
	resp3, html3 := server.get(t, "/admin")
	assertHTTPStatus(t, resp3, 200)
	if !strings.Contains(html3, "Login") {
		t.Error("expected client page to contain 'Login'")
	}

	matchSnapshot(t, "mixed_all_three_dashboard_dev", html1)
	matchSnapshot(t, "mixed_all_three_about_dev", html2)
	matchSnapshot(t, "mixed_all_three_admin_dev", html3)
}

// TestMixedMode_AllThree_Prod tests an app with SSR, Static, and Client pages in prod mode
// Uses example app routes: /dashboard (SSR), /product (Static), /about (Client)
func TestMixedMode_AllThree_Prod(t *testing.T) {
	skipIfNoBun(t)

	// Use example app's actual routes
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

	// Test SSR page (/dashboard)
	resp1, html1 := server.get(t, "/dashboard?demo=true")
	assertHTTPStatus(t, resp1, 200)

	// Test static page (/product)
	resp2, html2 := server.get(t, "/product")
	assertHTTPStatus(t, resp2, 200)

	// Test client page (/about)
	resp3, html3 := server.get(t, "/about")
	assertHTTPStatus(t, resp3, 200)

	matchSnapshot(t, "mixed_all_three_dashboard_prod", html1)
	matchSnapshot(t, "mixed_all_three_product_prod", html2)
	matchSnapshot(t, "mixed_all_three_about_prod", html3)
}

// TestMixedMode_SSRAndStatic_Dev tests SSR + Static pages (no client) in dev mode
func TestMixedMode_SSRAndStatic_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/", "./pages/home.tsx", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{"name": "Home"}, nil
		})),
		bifrost.Page("/product", "./pages/product.tsx", bifrost.WithStatic()),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp1, html1 := server.get(t, "/")
	assertHTTPStatus(t, resp1, 200)
	if !strings.Contains(html1, "Home") {
		t.Error("expected SSR page to contain 'Home'")
	}

	resp2, html2 := server.get(t, "/product")
	assertHTTPStatus(t, resp2, 200)

	matchSnapshot(t, "mixed_ssr_and_static_home_dev", html1)
	matchSnapshot(t, "mixed_ssr_and_static_product_dev", html2)
}

// TestMixedMode_SSRAndStatic_Prod tests SSR + Static pages (no client) in prod mode
func TestMixedMode_SSRAndStatic_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/", "./pages/home.tsx", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{"name": "Home"}, nil
		})),
		bifrost.Page("/product", "./pages/product.tsx", bifrost.WithStatic()),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	resp1, html1 := server.get(t, "/")
	assertHTTPStatus(t, resp1, 200)
	if !strings.Contains(html1, "Home") {
		t.Error("expected SSR page to contain 'Home'")
	}

	resp2, html2 := server.get(t, "/product")
	assertHTTPStatus(t, resp2, 200)

	matchSnapshot(t, "mixed_ssr_and_static_home_prod", html1)
	matchSnapshot(t, "mixed_ssr_and_static_product_prod", html2)
}

// TestMixedMode_StaticAndClient_Dev tests Static + Client pages (no SSR) in dev mode
// This is the CRITICAL test - verifies website pattern works without runtime
func TestMixedMode_StaticAndClient_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/about", "./pages/about.tsx", bifrost.WithStatic()),
		bifrost.Page("/login", "./pages/login.tsx", bifrost.WithClient()),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp1, html1 := server.get(t, "/about")
	assertHTTPStatus(t, resp1, 200)
	if !strings.Contains(html1, "About") {
		t.Error("expected static page to contain 'About'")
	}

	resp2, html2 := server.get(t, "/login")
	assertHTTPStatus(t, resp2, 200)
	if !strings.Contains(html2, "Login") {
		t.Error("expected client page to contain 'Login'")
	}

	matchSnapshot(t, "mixed_static_and_client_about_dev", html1)
	matchSnapshot(t, "mixed_static_and_client_login_dev", html2)
}

// TestMixedMode_StaticAndClient_Prod tests Static + Client pages (no SSR) in prod mode
// Uses example app routes: /product (Static), /about (Client)
// This is the CRITICAL test - verifies website pattern works without runtime
func TestMixedMode_StaticAndClient_Prod(t *testing.T) {
	skipIfNoBun(t)

	// Use example app's actual routes
	routes := []bifrost.Route{
		bifrost.Page("/product", "./pages/product.tsx", bifrost.WithStatic()),
		bifrost.Page("/about", "./pages/about.tsx", bifrost.WithClient()),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	// Test static page (/product)
	resp1, html1 := server.get(t, "/product")
	assertHTTPStatus(t, resp1, 200)

	// Test client page (/about)
	resp2, html2 := server.get(t, "/about")
	assertHTTPStatus(t, resp2, 200)

	matchSnapshot(t, "mixed_static_and_client_product_prod", html1)
	matchSnapshot(t, "mixed_static_and_client_about_prod", html2)
}

// TestMixedMode_SSRAndClient_Dev tests SSR + Client pages (no static) in dev mode
func TestMixedMode_SSRAndClient_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/app", "./pages/home.tsx", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{"name": "App"}, nil
		})),
		bifrost.Page("/login", "./pages/login.tsx", bifrost.WithClient()),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp1, html1 := server.get(t, "/app")
	assertHTTPStatus(t, resp1, 200)
	if !strings.Contains(html1, "App") {
		t.Error("expected SSR page to contain 'App'")
	}

	resp2, html2 := server.get(t, "/login")
	assertHTTPStatus(t, resp2, 200)
	if !strings.Contains(html2, "Login") {
		t.Error("expected client page to contain 'Login'")
	}

	matchSnapshot(t, "mixed_ssr_and_client_app_dev", html1)
	matchSnapshot(t, "mixed_ssr_and_client_login_dev", html2)
}

// TestMixedMode_SSRAndClient_Prod tests SSR + Client pages (no static) in prod mode
func TestMixedMode_SSRAndClient_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/app", "./pages/home.tsx", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{"name": "App"}, nil
		})),
		bifrost.Page("/login", "./pages/login.tsx", bifrost.WithClient()),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	resp1, html1 := server.get(t, "/app")
	assertHTTPStatus(t, resp1, 200)
	if !strings.Contains(html1, "App") {
		t.Error("expected SSR page to contain 'App'")
	}

	resp2, html2 := server.get(t, "/login")
	assertHTTPStatus(t, resp2, 200)
	if !strings.Contains(html2, "Login") {
		t.Error("expected client page to contain 'Login'")
	}

	matchSnapshot(t, "mixed_ssr_and_client_app_prod", html1)
	matchSnapshot(t, "mixed_ssr_and_client_login_prod", html2)
}

// TestMixedMode_MultipleEach_Dev tests multiple pages of each type in dev mode
func TestMixedMode_MultipleEach_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		// Multiple SSR
		bifrost.Page("/dashboard", "./pages/home.tsx", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{"name": "Dashboard"}, nil
		})),
		bifrost.Page("/profile", "./pages/home.tsx", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{"name": "Profile"}, nil
		})),
		// Multiple Static
		bifrost.Page("/about", "./pages/about.tsx", bifrost.WithStatic()),
		bifrost.Page("/pricing", "./pages/about.tsx", bifrost.WithStatic()),
		// Multiple Client
		bifrost.Page("/admin", "./pages/login.tsx", bifrost.WithClient()),
		bifrost.Page("/settings", "./pages/login.tsx", bifrost.WithClient()),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	// Test all SSR pages
	resp1, _ := server.get(t, "/dashboard")
	assertHTTPStatus(t, resp1, 200)
	resp2, _ := server.get(t, "/profile")
	assertHTTPStatus(t, resp2, 200)

	// Test all static pages
	resp3, _ := server.get(t, "/about")
	assertHTTPStatus(t, resp3, 200)
	resp4, _ := server.get(t, "/pricing")
	assertHTTPStatus(t, resp4, 200)

	// Test all client pages
	resp5, _ := server.get(t, "/admin")
	assertHTTPStatus(t, resp5, 200)
	resp6, _ := server.get(t, "/settings")
	assertHTTPStatus(t, resp6, 200)
}

// TestMixedMode_MultipleEach_Prod tests multiple pages of each type in prod mode
// Uses example app routes: / (SSR), /simple (SSR), /product (Static), /about (Client), /login (Client)
func TestMixedMode_MultipleEach_Prod(t *testing.T) {
	skipIfNoBun(t)

	// Use example app's actual routes
	routes := []bifrost.Route{
		// Multiple SSR pages
		bifrost.Page("/{$}", "./pages/home.tsx", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{"name": "Home"}, nil
		})),
		bifrost.Page("/simple", "./pages/home.tsx"),
		// Static page
		bifrost.Page("/product", "./pages/product.tsx", bifrost.WithStatic()),
		// Multiple Client pages
		bifrost.Page("/about", "./pages/about.tsx", bifrost.WithClient()),
		bifrost.Page("/login", "./pages/login.tsx", bifrost.WithClient()),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	// Test all SSR pages
	resp1, _ := server.get(t, "/")
	assertHTTPStatus(t, resp1, 200)
	resp2, _ := server.get(t, "/simple")
	assertHTTPStatus(t, resp2, 200)

	// Test static page
	resp3, _ := server.get(t, "/product")
	assertHTTPStatus(t, resp3, 200)

	// Test all client pages
	resp4, _ := server.get(t, "/about")
	assertHTTPStatus(t, resp4, 200)
	resp5, _ := server.get(t, "/login")
	assertHTTPStatus(t, resp5, 200)
}
