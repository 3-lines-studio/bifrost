package e2e

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/3-lines-studio/bifrost"
)

type authRequiredError struct{}

func (e *authRequiredError) Error() string {
	return "authentication required"
}

func (e *authRequiredError) RedirectURL() string {
	return "/login"
}

func (e *authRequiredError) RedirectStatusCode() int {
	return 302
}

func TestRedirectError_Unauthenticated_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/dashboard", "./pages/dashboard.tsx", bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
			return nil, &authRequiredError{}
		})),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	assertRedirect(t, server.url("/dashboard"), "/login", 302)
}

func TestRedirectError_Unauthenticated_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/dashboard", "./pages/dashboard.tsx", bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
			return nil, &authRequiredError{}
		})),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	assertRedirect(t, server.url("/dashboard"), "/login", 302)
}

func TestRedirectError_Authenticated_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/dashboard", "./pages/dashboard.tsx", bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
			if req.URL.Query().Get("demo") == "true" {
				return map[string]any{
					"user": map[string]string{
						"name": "TestUser",
						"role": "Admin",
					},
				}, nil
			}
			return nil, &authRequiredError{}
		})),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/dashboard?demo=true")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "redirect_authenticated_dev", html)
}

func TestRedirectError_Authenticated_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/dashboard", "./pages/dashboard.tsx", bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
			if req.URL.Query().Get("demo") == "true" {
				return map[string]any{
					"user": map[string]string{
						"name": "ProdUser",
						"role": "Manager",
					},
				}, nil
			}
			return nil, &authRequiredError{}
		})),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/dashboard?demo=true")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "redirect_authenticated_prod", html)
}

func TestErrorLoaderError_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/error", "./pages/home.tsx", bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
			return nil, fmt.Errorf("loader error occurred")
		})),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/error")
	assertHTTPStatus(t, resp, 500)

	matchSnapshot(t, "error_loader_dev", html)
}

func TestErrorLoaderError_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/error", "./pages/home.tsx", bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
			return nil, fmt.Errorf("loader error occurred")
		})),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/error")
	assertHTTPStatus(t, resp, 500)

	matchSnapshot(t, "error_loader_prod", html)
}

func TestErrorRenderError_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/error-render", "./pages/error-render.tsx"),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/error-render")
	assertHTTPStatus(t, resp, 500)

	matchSnapshot(t, "error_render_dev", html)
}

func TestErrorRenderError_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/error-render", "./pages/error-render.tsx"),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/error-render")
	assertHTTPStatus(t, resp, 500)

	matchSnapshot(t, "error_render_prod", html)
}

func TestErrorImportError_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/error-import", "./pages/error-import.tsx"),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/error-import")
	assertHTTPStatus(t, resp, 500)

	matchSnapshot(t, "error_import_dev", html)
}

func TestErrorImportError_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/error-import", "./pages/error-import.tsx"),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/error-import")
	assertHTTPStatus(t, resp, 500)

	matchSnapshot(t, "error_import_prod", html)
}

type adminRequiredError struct{}

func (e *adminRequiredError) Error() string {
	return "admin access required"
}

func (e *adminRequiredError) RedirectURL() string {
	return "/unauthorized"
}

func (e *adminRequiredError) RedirectStatusCode() int {
	return http.StatusTemporaryRedirect
}

func TestRedirectError_307_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/admin", "./pages/dashboard.tsx", bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
			return nil, &adminRequiredError{}
		})),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	assertRedirect(t, server.url("/admin"), "/unauthorized", 307)
}

func TestRedirectError_307_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/admin", "./pages/dashboard.tsx", bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
			return nil, &adminRequiredError{}
		})),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	assertRedirect(t, server.url("/admin"), "/unauthorized", 307)
}

func TestNotFound_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/exists", "./pages/home.tsx"),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp, _ := server.get(t, "/does-not-exist")
	assertHTTPStatus(t, resp, 404)
}

func TestNotFound_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/exists", "./pages/home.tsx"),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	resp, _ := server.get(t, "/does-not-exist")
	assertHTTPStatus(t, resp, 404)
}
