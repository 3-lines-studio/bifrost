package e2e

import (
	"net/http"
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost"
)

func TestSvelteSSRPage_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/svelte", "./pages/svelte/hello.svelte", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{"name": "Svelte"}, nil
		})),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/svelte")
	assertHTTPStatus(t, resp, 200)

	if !strings.Contains(html, "Svelte 5") {
		t.Error("expected HTML to contain 'Svelte 5'")
	}

	matchSnapshot(t, "svelte_ssr_dev", html)
}

func TestSvelteSSRPage_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/svelte", "./pages/svelte/hello.svelte", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{"name": "Svelte"}, nil
		})),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/svelte")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_ssr_prod", html)
}

func TestSvelteClientOnlyPage_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/svelte-client", "./pages/svelte/counter.svelte", bifrost.WithClient()),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/svelte-client")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_client_dev", html)
}

func TestSvelteClientOnlyPage_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/svelte-client", "./pages/svelte/counter.svelte", bifrost.WithClient()),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/svelte-client")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_client_prod", html)
}

func TestSvelteNestedComponents_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/svelte", "./pages/svelte/hello.svelte", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{"name": "Svelte"}, nil
		})),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/svelte")
	assertHTTPStatus(t, resp, 200)

	if !strings.Contains(html, "Nested Components") {
		t.Error("expected HTML to contain 'Nested Components'")
	}

	if !strings.Contains(html, "Static value") {
		t.Error("expected HTML to contain 'Static value'")
	}

	matchSnapshot(t, "svelte_nested_dev", html)
}

func TestSvelteBrokenPage_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/svelte-broken", "./pages/svelte/broken.svelte"),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/svelte-broken")
	assertHTTPStatus(t, resp, 500)

	if !strings.Contains(html, "expected") {
		t.Error("expected error HTML to contain Svelte parser hint")
	}

	matchSnapshot(t, "svelte_broken_dev", html)
}

func TestSvelteStaticPage_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/svelte-static", "./pages/svelte/static.svelte", bifrost.WithStatic()),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/svelte-static")
	assertHTTPStatus(t, resp, 200)

	if !strings.Contains(html, "Svelte Static") {
		t.Error("expected HTML to contain 'Svelte Static'")
	}

	matchSnapshot(t, "svelte_static_dev", html)
}

func TestSvelteStaticPage_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/svelte-static", "./pages/svelte/static.svelte", bifrost.WithStatic()),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/svelte-static")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_static_prod", html)
}

func TestSvelteNestedComponents_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/svelte", "./pages/svelte/hello.svelte", bifrost.WithLoader(func(r *http.Request) (map[string]any, error) {
			return map[string]any{"name": "Svelte"}, nil
		})),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/svelte")
	assertHTTPStatus(t, resp, 200)

	matchSnapshot(t, "svelte_nested_prod", html)
}
