package e2e

import (
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost"
)

func TestStreamReactBodySSR_Dev(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/stream-demo", "./pages/stream-demo.tsx"),
	}

	server := newTestServer(t, routes, true)
	server.start(t)

	resp, html := server.get(t, "/stream-demo")
	assertHTTPStatus(t, resp, 200)
	if !strings.Contains(html, "stream-demo-root") {
		t.Fatalf("expected stream demo markup in HTML, got snippet: %q", truncate(html, 200))
	}
}

func TestStreamReactBodySSR_Prod(t *testing.T) {
	skipIfNoBun(t)

	routes := []bifrost.Route{
		bifrost.Page("/stream-demo", "./pages/stream-demo.tsx"),
	}

	server := newTestServer(t, routes, false)
	server.start(t)

	resp, html := server.get(t, "/stream-demo")
	assertHTTPStatus(t, resp, 200)
	if !strings.Contains(html, "stream-demo-root") {
		t.Fatalf("expected stream demo markup in HTML, got snippet: %q", truncate(html, 200))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
