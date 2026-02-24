package e2e

import (
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost"
)

func TestClientOnlyAboutPage_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/about", "./pages/about.tsx", bifrost.WithClient()),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/about")
	assertHTTPStatus(t, resp, 200)

	if !strings.Contains(html, "About") {
		t.Error("expected HTML to contain 'About'")
	}

	matchSnapshot(t, "client_about_dev", html)
}

func TestClientOnlyAboutPage_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/about", "./pages/about.tsx", bifrost.WithClient()),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/about")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "client_about_prod", html)
}

func TestClientOnlyLoginPage_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/login", "./pages/login.tsx", bifrost.WithClient()),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/login")
	assertHTTPStatus(t, resp, 200)

	if !strings.Contains(html, "Login") {
		t.Error("expected HTML to contain 'Login'")
	}

	matchSnapshot(t, "client_login_dev", html)
}

func TestClientOnlyLoginPage_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/login", "./pages/login.tsx", bifrost.WithClient()),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/login")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "client_login_prod", html)
}

func TestClientOnlyNestedPath_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/client/deep", "./pages/login.tsx", bifrost.WithClient()),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/client/deep")
	assertHTTPStatus(t, resp, 200)

	if !strings.Contains(html, "Login") {
		t.Error("expected HTML to contain 'Login'")
	}

	matchSnapshot(t, "client_nested_dev", html)
}

func TestClientOnlyNestedPath_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/client/deep", "./pages/login.tsx", bifrost.WithClient()),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/client/deep")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "client_nested_prod", html)
}
