package http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
