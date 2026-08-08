package http

import (
	"context"
	"embed"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/3-lines-studio/bifrost/internal/core"
	"github.com/3-lines-studio/bifrost/internal/usecase"
)

func TestComputeNextSteps_BuildErrorWithSpecifier(t *testing.T) {
	se := &core.StructuredError{
		ErrorType: "Build Error",
		Message:   "Build failed",
		Specifier: "invalid-import",
		File:      "src/App.tsx",
		Line:      3,
		Column:    21,
		SubErrors: []core.StructuredError{
			{
				Message:   "Could not resolve: \"invalid-import\"",
				File:      "src/App.tsx",
				Line:      3,
				Column:    21,
				Specifier: "invalid-import",
			},
		},
	}

	steps := computeNextSteps(se)
	found := false
	for _, s := range steps {
		if strings.Contains(s, "invalid-import") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected next step to mention the import specifier, got: %v", steps)
	}
}

func TestComputeNextSteps_BuildErrorWithFileLine(t *testing.T) {
	se := &core.StructuredError{
		ErrorType: "Build Error",
		Message:   "Build failed",
		File:      "src/App.tsx",
		Line:      2,
		Column:    1,
		SubErrors: []core.StructuredError{
			{
				Message: "Expected whitespace",
				File:    "src/App.tsx",
				Line:    2,
				Column:  1,
			},
		},
	}

	steps := computeNextSteps(se)
	if len(steps) == 0 || !strings.Contains(steps[0], "src/App.tsx:2") {
		t.Errorf("expected first next step to point to the error location, got: %v", steps)
	}
}

func TestComputeNextSteps_RenderError(t *testing.T) {
	se := &core.StructuredError{
		ErrorType: "Render Error",
		Message:   "Render error",
	}

	steps := computeNextSteps(se)
	if len(steps) != 1 || !strings.Contains(steps[0], "component rendering logic") {
		t.Errorf("expected render-specific next step, got: %v", steps)
	}
}

func TestServeError_StructuredErrorThroughFmtWrap(t *testing.T) {
	se := &core.StructuredError{
		ErrorType: "Render Error",
		Message:   "Failed to import component",
		Stack:     "Error: test at Page (file.tsx:1:1)",
	}
	wrapped := fmt.Errorf("outer: %w", se)

	h := &PageHandler{isDev: true}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	h.serveError(rec, req, wrapped)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Render Error") {
		t.Errorf("expected structured error type badge in dev error page, got: %s", body)
	}
	if !strings.Contains(body, "Failed to import component") {
		t.Errorf("expected structured error message in dev error page, got: %s", body)
	}
}

func TestServeError_ProductionHidesStructuredDetails(t *testing.T) {
	se := &core.StructuredError{
		ErrorType: "Render Error",
		Message:   "Failed to import component",
		Stack:     "Error: test at Page (file.tsx:1:1)",
	}

	h := &PageHandler{isDev: false}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	h.serveError(rec, req, se)

	body := rec.Body.String()
	if strings.Contains(body, "Failed to import component") {
		t.Errorf("production error page should not leak structured error message, got: %s", body)
	}
	if !strings.Contains(body, "An error occurred while processing your request") {
		t.Errorf("expected generic production message, got: %s", body)
	}
}

func TestServeError_RedirectErrorTakesPrecedence(t *testing.T) {
	redirectErr := &testRedirectErr{url: "/login", code: http.StatusFound}

	h := &PageHandler{isDev: true}
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()

	h.serveError(rec, req, fmt.Errorf("load user: %w", redirectErr))

	if rec.Code != http.StatusFound {
		t.Errorf("expected redirect status %d, got %d", http.StatusFound, rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != "/login" {
		t.Errorf("expected redirect to /login, got %s", loc)
	}
}

type testRedirectErr struct {
	url  string
	code int
}

func TestServeError_InvalidRedirectStatusReturns500(t *testing.T) {
	for _, status := range []int{99, http.StatusMultipleChoices, http.StatusNotModified, http.StatusUseProxy, 306, 399} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			h := &PageHandler{}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/private", nil)

			h.serveError(rec, req, &testRedirectErr{url: "/login", code: status})

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
			}
		})
	}
}

func TestRedirectStatusAllowsHTTPRedirectCodes(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		if !isRedirectStatus(status) {
			t.Errorf("status %d was rejected", status)
		}
	}
}

func (e *testRedirectErr) Error() string           { return "redirect" }
func (e *testRedirectErr) RedirectURL() string     { return e.url }
func (e *testRedirectErr) RedirectStatusCode() int { return e.code }

var _ core.RedirectError = (*testRedirectErr)(nil)
var _ error = (*testRedirectErr)(nil)

func TestResolveMarkdown_SuffixDetection(t *testing.T) {
	cases := []struct {
		name            string
		urlPath         string
		wantMarkdown    bool
		wantRequestPath string
	}{
		{name: "lowercase md", urlPath: "/about.md", wantMarkdown: true, wantRequestPath: "/about"},
		{name: "uppercase MD", urlPath: "/ABOUT.MD", wantMarkdown: true, wantRequestPath: "/ABOUT"},
		{name: "mixed case Md", urlPath: "/post.Md", wantMarkdown: true, wantRequestPath: "/post"},
		{name: "mixed case mD", urlPath: "/post.mD", wantMarkdown: true, wantRequestPath: "/post"},
		{name: "nested path md", urlPath: "/blog/hello-world.md", wantMarkdown: true, wantRequestPath: "/blog/hello-world"},
		{name: "no suffix", urlPath: "/about", wantMarkdown: false, wantRequestPath: "/about"},
		{name: "html suffix", urlPath: "/about.html", wantMarkdown: false, wantRequestPath: "/about.html"},
		{name: "dotfile not md", urlPath: "/.bashrc", wantMarkdown: false, wantRequestPath: "/.bashrc"},
		{name: "suffix in middle", urlPath: "/about.md/info", wantMarkdown: false, wantRequestPath: "/about.md/info"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			var gotMarkdown bool
			handler := ResolveMarkdown(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotMarkdown, _ = r.Context().Value(markdownCtxKey{}).(bool)
			}))
			req := httptest.NewRequest(http.MethodGet, tc.urlPath, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if gotPath != tc.wantRequestPath {
				t.Errorf("RequestPath = %q, want %q", gotPath, tc.wantRequestPath)
			}
			if gotMarkdown != tc.wantMarkdown {
				t.Errorf("Markdown = %v, want %v", gotMarkdown, tc.wantMarkdown)
			}
		})
	}
}

func TestResolveMarkdown_AcceptHeader(t *testing.T) {
	cases := []struct {
		name         string
		urlPath      string
		accept       string
		wantMarkdown bool
	}{
		{name: "accept text/markdown", urlPath: "/about", accept: "text/markdown", wantMarkdown: true},
		{name: "accept with q-value", urlPath: "/about", accept: "text/markdown;q=0.9", wantMarkdown: true},
		{name: "accept mixed types", urlPath: "/about", accept: "text/markdown, text/html", wantMarkdown: true},
		{name: "accept disabled by q zero", urlPath: "/about", accept: "text/markdown;q=0, text/html", wantMarkdown: false},
		{name: "accept ignores substring", urlPath: "/about", accept: "application/text/markdownish", wantMarkdown: false},
		{name: "accept text/html only", urlPath: "/about", accept: "text/html", wantMarkdown: false},
		{name: "no accept header", urlPath: "/about", accept: "", wantMarkdown: false},
		{name: "accept header plus suffix", urlPath: "/about.md", accept: "text/html", wantMarkdown: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotMarkdown bool
			handler := ResolveMarkdown(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMarkdown, _ = r.Context().Value(markdownCtxKey{}).(bool)
			}))
			req := httptest.NewRequest(http.MethodGet, tc.urlPath, nil)
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if gotMarkdown != tc.wantMarkdown {
				t.Errorf("Markdown = %v, want %v", gotMarkdown, tc.wantMarkdown)
			}
		})
	}
}

func TestServePageInput_ReadsMarkdownFromContext(t *testing.T) {
	h := &PageHandler{}
	req := httptest.NewRequest(http.MethodGet, "/about", nil)
	ctx := context.WithValue(req.Context(), markdownCtxKey{}, true)
	req = req.WithContext(ctx)
	input := h.servePageInput(req)
	if !input.Markdown {
		t.Errorf("expected Markdown = true from context")
	}
	if input.RequestPath != "/about" {
		t.Errorf("RequestPath = %q, want %q", input.RequestPath, "/about")
	}
}

func TestDispatchPageOutput_ServeMarkdownContentType(t *testing.T) {
	h := &PageHandler{}
	req := httptest.NewRequest(http.MethodGet, "/about.md", nil)
	rec := httptest.NewRecorder()

	h.dispatchPageOutput(rec, req, usecase.ServePageOutput{
		Action:     core.ActionRenderSSR,
		Markdown:   "# About\n\nSome content.",
		IsMarkdown: true,
	})

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "text/markdown; charset=utf-8" {
		t.Errorf("expected content-type %q, got %q", "text/markdown; charset=utf-8", ct)
	}
	if rec.Body.String() != "# About\n\nSome content." {
		t.Errorf("unexpected body: %q", rec.Body.String())
	}
}

func TestDispatchPageOutput_ServesEmptyMarkdown(t *testing.T) {
	h := &PageHandler{}
	req := httptest.NewRequest(http.MethodGet, "/about.md", nil)
	rec := httptest.NewRecorder()

	h.dispatchPageOutput(rec, req, usecase.ServePageOutput{
		Action:     core.ActionRenderSSR,
		IsMarkdown: true,
	})

	ct := rec.Header().Get("Content-Type")
	if ct != "text/markdown; charset=utf-8" {
		t.Errorf("expected markdown content type, got %q", ct)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty markdown body, got %q", rec.Body.String())
	}
}

func TestSetTimingHeaders_RenderPathSetsAllSpans(t *testing.T) {
	h := &PageHandler{}
	rec := httptest.NewRecorder()

	h.setTimingHeaders(rec, usecase.ServePageOutput{
		RenderMs:   12.3,
		PropsMs:    250.5,
		AssembleMs: 4.2,
	}, 300*time.Millisecond, 1.5, true)

	want := map[string]string{
		"X-Bifrost-Render-Ms":    "12.3",
		"X-Bifrost-Props-Ms":     "250.5",
		"X-Bifrost-Assemble-Ms":  "4.2",
		"X-Bifrost-PreLoader-Ms": "1.5",
		"X-Bifrost-Serve-Ms":     "300.0",
	}
	for name, value := range want {
		if got := rec.Header().Get(name); got != value {
			t.Errorf("header %s = %q, want %q", name, got, value)
		}
	}
	if got := rec.Header().Get("Server-Timing"); !strings.Contains(got, "bifrost-render;dur=12.3") {
		t.Errorf("Server-Timing = %q, want render span", got)
	}
}

func TestSendEarlyHints_IncludesPageAndDynamicPreloads(t *testing.T) {
	h := &PageHandler{
		config: core.PageConfig{Preloads: []core.Preload{
			{Kind: core.Preconnect, Href: "https://img.example.com"},
			{Kind: core.PreloadLink, Href: "/logo.svg", As: "image"},
		}},
	}
	rec := httptest.NewRecorder()

	h.sendEarlyHints(rec, &core.PreLoaderResult{Preloads: []core.Preload{
		{Kind: core.PreloadLink, Href: "/hero.webp", As: "image", FetchPriority: "high"},
	}})

	want := []string{
		"<https://img.example.com>; rel=preconnect",
		"</logo.svg>; rel=preload; as=image",
		"</hero.webp>; rel=preload; as=image; fetchpriority=high",
	}
	links := rec.Header().Values("Link")
	for _, w := range want {
		if !slices.Contains(links, w) {
			t.Errorf("missing Link %q in %v", w, links)
		}
	}
}

func TestIsBotUA(t *testing.T) {
	cases := []struct {
		ua   string
		want bool
	}{
		{"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", true},
		{"Twitterbot/1.0", true},
		{"facebookexternalhit/1.1", true},
		{"curl/8.0.0", false},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isBotUA(c.ua); got != c.want {
			t.Errorf("isBotUA(%q) = %v, want %v", c.ua, got, c.want)
		}
	}
}

func TestServeHTTP_StreamsExtrasInHead(t *testing.T) {
	config := core.PageConfig{
		Mode:           core.ModeSSR,
		ComponentPath:  "./pages/home.tsx",
		StreamingShell: ".shell{position:fixed;inset:0}",
		PrerenderPaths: []string{"/comprar-fotos"},
		Preloads: []core.Preload{
			{Kind: core.PreloadLink, Href: "/hero.webp", As: "image", FetchPriority: "high"},
			{Kind: core.Preconnect, Href: "https://img.example.com"},
		},
	}
	entryName := core.EntryNameForPath(config.ComponentPath)
	manifest := &core.Manifest{Entries: map[string]core.ManifestEntry{
		entryName: {Script: "/dist/page.js"},
	}}
	handler := NewPageHandler(usecase.NewPageService(stubRenderer{}), config, manifest, embed.FS{}, false, "", "en")

	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		`<link rel="preload" href="/hero.webp" as="image" fetchpriority="high" />`,
		`<link rel="preconnect" href="https://img.example.com" />`,
		`<style id="bifrost-shell">.shell{position:fixed;inset:0}</style>`,
		`<script type="speculationrules">`,
		`"urls":["/comprar-fotos"]`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("streamed head missing %q:\n%s", want, s)
		}
	}
	if !strings.Contains(strings.Join(resp.Header.Values("Link"), " "), "/hero.webp") {
		t.Errorf("Link header missing preload hint, got %v", resp.Header.Values("Link"))
	}
	if got := resp.Trailer.Get("Server-Timing"); got == "" {
		t.Error("missing Server-Timing trailer")
	}
}

func TestSetTimingHeaders_SkipsNonRenderPaths(t *testing.T) {
	h := &PageHandler{}
	rec := httptest.NewRecorder()

	h.setTimingHeaders(rec, usecase.ServePageOutput{}, time.Millisecond, 0, false)

	for _, name := range []string{"X-Bifrost-Render-Ms", "X-Bifrost-Props-Ms", "X-Bifrost-Assemble-Ms", "X-Bifrost-PreLoader-Ms", "X-Bifrost-Serve-Ms"} {
		if got := rec.Header().Get(name); got != "" {
			t.Errorf("unexpected header %s = %q", name, got)
		}
	}
}

func TestEarlyHintLinks(t *testing.T) {
	a := core.PageArtifacts{
		Script:      "/dist/page.js",
		CriticalCSS: ".a{}",
		CSS:         "/dist/page.css",
		CSSFiles:    []string{"/dist/extra.css"},
		Chunks:      []string{"/dist/chunk-a.js"},
	}
	want := []string{
		"</dist/page.css>; rel=preload; as=style",
		"</dist/extra.css>; rel=preload; as=style",
		"</dist/chunk-a.js>; rel=modulepreload",
		"</dist/page.js>; rel=modulepreload",
	}
	if got := earlyHintLinks(a); !slices.Equal(got, want) {
		t.Errorf("earlyHintLinks = %v, want %v", got, want)
	}
}

func TestSendEarlyHints_SetsLinkHeaders(t *testing.T) {
	h := &PageHandler{earlyHints: []string{"</dist/page.css>; rel=preload; as=style"}}
	rec := httptest.NewRecorder()

	h.sendEarlyHints(rec, nil)

	if got := rec.Header().Get("Link"); got != "</dist/page.css>; rel=preload; as=style" {
		t.Errorf("Link header = %q, want early hint link", got)
	}
}

func TestServeStreaming_WritesFullDocument(t *testing.T) {
	shell, err := core.NewHTMLDocumentShell("/dist/page.js", "", nil, []string{"/dist/chunk-a.js"})
	if err != nil {
		t.Fatal(err)
	}
	h := &PageHandler{shell: &shell}
	rec := httptest.NewRecorder()

	h.writeStreamingHead(rec, nil)
	h.serveStreamingTail(rec, usecase.ServePageOutput{
		Action: core.ActionRenderSSR,
		Page:   core.RenderedPage{Head: "<title>Stub</title>", Body: "<main>stub</main>"},
		Props:  map[string]any{"name": "World"},
	})

	body := rec.Body.String()
	for _, want := range []string{"<!doctype html>", `rel="modulepreload"`, "<title>Stub</title>", "<main>stub</main>", `"name":"World"`, `type="module" defer`} {
		if !strings.Contains(body, want) {
			t.Errorf("streamed body missing %q:\n%s", want, body)
		}
	}
}

func TestServeStreamingError_RedirectUsesMetaRefresh(t *testing.T) {
	shell, err := core.NewHTMLDocumentShell("/dist/page.js", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := &PageHandler{shell: &shell, isDev: true}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	h.serveStreamingError(rec, req, &testRedirectErr{url: "/login", code: http.StatusFound})

	body := rec.Body.String()
	if !strings.Contains(body, `<meta http-equiv="refresh" content="0; url=/login">`) {
		t.Errorf("expected meta refresh redirect, got:\n%s", body)
	}
	if !strings.Contains(body, `href="/login"`) {
		t.Errorf("expected link to /login, got:\n%s", body)
	}
}

func TestServeStreamingError_ProdHidesDetails(t *testing.T) {
	shell, err := core.NewHTMLDocumentShell("/dist/page.js", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := &PageHandler{shell: &shell, isDev: false}
	rec := httptest.NewRecorder()

	h.serveStreamingError(rec, httptest.NewRequest(http.MethodGet, "/", nil), fmt.Errorf("secret detail"))

	body := rec.Body.String()
	if strings.Contains(body, "secret detail") {
		t.Errorf("production must not leak error details, got:\n%s", body)
	}
	if !strings.Contains(body, "An error occurred while processing your request") {
		t.Errorf("expected generic production message, got:\n%s", body)
	}
}

type stubRenderer struct{}

func (stubRenderer) Render(path string, props any) (core.RenderedPage, error) {
	return core.RenderedPage{Head: "<title>Stub</title>", Body: "<main>stub</main>"}, nil
}

func (stubRenderer) Build(entrypoints []string, outdir string, entryNames []string) (map[string]core.ClientBuildResult, error) {
	return nil, nil
}

func (stubRenderer) BuildSSR(entrypoints []string, outdir string) error {
	return nil
}

func TestServeHTTP_StreamsSSRPage(t *testing.T) {
	config := core.PageConfig{Mode: core.ModeSSR, ComponentPath: "./pages/home.tsx"}
	entryName := core.EntryNameForPath(config.ComponentPath)
	manifest := &core.Manifest{Entries: map[string]core.ManifestEntry{
		entryName: {Script: "/dist/page.js"},
	}}
	handler := NewPageHandler(usecase.NewPageService(stubRenderer{}), config, manifest, embed.FS{}, false, "", "en")

	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("streamed response Content-Type = %q, want text/html; charset=utf-8", ct)
	}
	for _, want := range []string{"<!doctype html>", "<title>Stub</title>", "<main>stub</main>", `src="/dist/page.js"`, `id="__BIFROST_PROPS__"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("streamed page missing %q:\n%s", want, body)
		}
	}
}

func TestServeHTTP_PreLoaderRedirectsBeforeFlush(t *testing.T) {
	config := core.PageConfig{
		Mode:          core.ModeSSR,
		ComponentPath: "./pages/home.tsx",
		PreLoader: func(r *http.Request) (core.PreLoaderResult, error) {
			return core.PreLoaderResult{}, &testRedirectErr{url: "/login", code: http.StatusFound}
		},
	}
	entryName := core.EntryNameForPath(config.ComponentPath)
	manifest := &core.Manifest{Entries: map[string]core.ManifestEntry{
		entryName: {Script: "/dist/page.js"},
	}}
	handler := NewPageHandler(usecase.NewPageService(stubRenderer{}), config, manifest, embed.FS{}, false, "", "en")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
	if strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Error("static head flushed before the pre loader redirect")
	}
}

func TestServeHTTP_PreLoaderLangInStreamedHead(t *testing.T) {
	config := core.PageConfig{
		Mode:          core.ModeSSR,
		ComponentPath: "./pages/home.tsx",
		PreLoader: func(r *http.Request) (core.PreLoaderResult, error) {
			return core.PreLoaderResult{Lang: "pt"}, nil
		},
	}
	entryName := core.EntryNameForPath(config.ComponentPath)
	manifest := &core.Manifest{Entries: map[string]core.ManifestEntry{
		entryName: {Script: "/dist/page.js"},
	}}
	handler := NewPageHandler(usecase.NewPageService(stubRenderer{}), config, manifest, embed.FS{}, false, "", "en")

	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), "<!doctype html>\n<html lang=\"pt\"") {
		t.Errorf("pre loader lang missing from streamed head:\n%s", body)
	}
	if got := resp.Header.Get("X-Bifrost-PreLoader-Ms"); got == "" {
		t.Error("missing X-Bifrost-PreLoader-Ms header")
	}
	if got := resp.Trailer.Get("X-Bifrost-Render-Ms"); got == "" {
		t.Error("missing X-Bifrost-Render-Ms trailer")
	}
}
