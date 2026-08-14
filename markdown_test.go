package bifrost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func markdownAppHandler(t *testing.T, render renderer) (*App, http.Handler) {
	t.Helper()
	app, state := runtimeFixture(t, render)
	app.runtime = state
	return app, app.Handler()
}

func markdownRenderer(body string) *fakeRenderer {
	return &fakeRenderer{render: func(_ context.Context, _ renderRequest, sink renderSink) error {
		if err := sink.Head([]byte(`<title>Page</title>`)); err != nil {
			return err
		}
		half := len(body) / 2
		if err := sink.Body([]byte(body[:half])); err != nil {
			return err
		}
		return sink.Body([]byte(body[half:]))
	}}
}

func TestServerHandlerServesMarkdownForAcceptHeader(t *testing.T) {
	_, handler := markdownAppHandler(t, markdownRenderer(`<h1>Hello</h1><p>Don Berti</p>`))

	request := httptest.NewRequest(http.MethodGet, "/server", nil)
	request.Header.Set("Accept", "text/markdown")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body:\n%s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/markdown; charset=utf-8" {
		t.Fatalf("content type = %q", contentType)
	}
	body := response.Body.String()
	for _, expected := range []string{"# Hello", "Don Berti"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("markdown body does not contain %q:\n%s", expected, body)
		}
	}
	if strings.Contains(body, "<") {
		t.Fatalf("markdown body still contains HTML:\n%s", body)
	}
}

func TestHandlerRewritesMarkdownSuffix(t *testing.T) {
	_, handler := markdownAppHandler(t, markdownRenderer(`<h1>Hello</h1>`))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/server.md", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body:\n%s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/markdown; charset=utf-8" {
		t.Fatalf("content type = %q", contentType)
	}
	if !strings.Contains(response.Body.String(), "# Hello") {
		t.Fatalf("body is not markdown:\n%s", response.Body.String())
	}
}

func TestHandlerServesHTMLWithoutMarkdownRequest(t *testing.T) {
	_, handler := markdownAppHandler(t, markdownRenderer(`<main>hello</main>`))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/server", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", contentType)
	}
	if !strings.Contains(response.Body.String(), `<!doctype html>`) {
		t.Fatalf("body is not an HTML document:\n%s", response.Body.String())
	}
}

func TestMarkdownRequestLeavesNonServerRoutesUnchanged(t *testing.T) {
	_, handler := markdownAppHandler(t, markdownRenderer(`<h1>Hello</h1>`))

	for _, path := range []string{"/client", "/static"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Accept", "text/markdown")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, response.Code)
		}
		if contentType := response.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
			t.Fatalf("%s content type = %q", path, contentType)
		}
	}

	for _, path := range []string{"/client.md", "/static.md"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d", path, response.Code)
		}
	}
}

func TestResolveMarkdownPreservesOwnedMarkdownPath(t *testing.T) {
	app, state := runtimeFixture(t, markdownRenderer(`<h1>Server</h1>`))
	app.runtime = state
	mux := http.NewServeMux()
	if err := app.Register(mux); err != nil {
		t.Fatal(err)
	}
	mux.HandleFunc("GET /server.md", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("owned"))
	})

	response := httptest.NewRecorder()
	app.ResolveMarkdown(mux).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/server.md", nil))
	if response.Code != http.StatusOK || response.Body.String() != "owned" {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
}

func TestResolveMarkdownPassesThroughMuxFallback(t *testing.T) {
	app, state := runtimeFixture(t, markdownRenderer(`<h1>Server</h1>`))
	app.runtime = state
	mux := http.NewServeMux()
	if err := app.Register(mux); err != nil {
		t.Fatal(err)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fallback", http.StatusBadGateway)
	})

	response := httptest.NewRecorder()
	app.ResolveMarkdown(mux).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/server.md", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body:\n%s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/markdown; charset=utf-8" {
		t.Fatalf("content type = %q", contentType)
	}
	if !strings.Contains(response.Body.String(), "# Server") {
		t.Fatalf("body is not markdown:\n%s", response.Body.String())
	}
}

func TestSubtreePattern(t *testing.T) {
	for _, test := range []struct {
		pattern string
		want    bool
	}{
		{pattern: "/", want: true},
		{pattern: "GET /api/", want: true},
		{pattern: "GET example.com/{rest...}", want: true},
		{pattern: "GET /policy.md", want: false},
		{pattern: "GET /files/{name}", want: false},
		{pattern: "GET /docs/{$}", want: false},
	} {
		if got := subtreePattern(test.pattern); got != test.want {
			t.Fatalf("subtreePattern(%q) = %t, want %t", test.pattern, got, test.want)
		}
	}
}

func TestResolveMarkdownPreservesEscapedPathAndQuery(t *testing.T) {
	app := &App{runtime: &runtimeState{serverPatterns: map[string]struct{}{"GET /files/{name}": {}}}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /files/{name}", func(w http.ResponseWriter, request *http.Request) {
		_, _ = w.Write([]byte(request.URL.EscapedPath() + " " + request.RequestURI + " " + request.PathValue("name")))
	})

	response := httptest.NewRecorder()
	app.ResolveMarkdown(mux).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/files/a%2Fb.md?lang=en", nil))
	want := "/files/a%2Fb /files/a%2Fb?lang=en a/b"
	if response.Body.String() != want {
		t.Fatalf("body = %q, want %q", response.Body.String(), want)
	}
}

func TestAcceptMarkdownNegotiation(t *testing.T) {
	for _, test := range []struct {
		header   string
		markdown bool
	}{
		{header: "text/markdown", markdown: true},
		{header: "Text/Markdown", markdown: true},
		{header: "text/markdown;q=0", markdown: false},
		{header: "text/markdown;q=0.8, text/html;q=1", markdown: false},
		{header: "text/markdown, text/html", markdown: true},
		{header: "*/*", markdown: false},
	} {
		t.Run(test.header, func(t *testing.T) {
			_, handler := markdownAppHandler(t, markdownRenderer(`<h1>Hello</h1>`))
			request := httptest.NewRequest(http.MethodGet, "/server", nil)
			request.Header.Set("Accept", test.header)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			contentType := response.Header().Get("Content-Type")
			if got := strings.HasPrefix(contentType, "text/markdown"); got != test.markdown {
				t.Fatalf("content type = %q", contentType)
			}
		})
	}
}

func TestDevelopmentStaticRouteDoesNotServeMarkdown(t *testing.T) {
	t.Setenv("BIFROST_DEV_DIR", t.TempDir())
	t.Setenv("BIFROST_VITE_PORT", "5173")
	_, handler := markdownAppHandler(t, markdownRenderer(`<h1>Static</h1>`))

	request := httptest.NewRequest(http.MethodGet, "/static", nil)
	request.Header.Set("Accept", "text/markdown")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if contentType := response.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", contentType)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/static.md", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestMarkdownRenderRejectsOversizedTotalBody(t *testing.T) {
	sink := &markdownRenderSink{writer: httptest.NewRecorder(), limits: Limits{MaxFrameBytes: 4, MaxMarkdownBytes: 6}}
	if err := sink.Head(nil); err != nil {
		t.Fatal(err)
	}
	if err := sink.Body([]byte("1234")); err != nil {
		t.Fatal(err)
	}
	if err := sink.Body([]byte("567")); err == nil || !strings.Contains(err.Error(), "markdown body exceeds 6 bytes") {
		t.Fatalf("error = %v", err)
	}
}

func TestServerHandlerServesErrorWhenMarkdownRenderFails(t *testing.T) {
	render := &fakeRenderer{render: func(_ context.Context, _ renderRequest, sink renderSink) error {
		return sink.Body([]byte(`<h1>Hello</h1>`))
	}}
	_, handler := markdownAppHandler(t, render)

	request := httptest.NewRequest(http.MethodGet, "/server", nil)
	request.Header.Set("Accept", "text/markdown")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body:\n%s", response.Code, response.Body.String())
	}
}
