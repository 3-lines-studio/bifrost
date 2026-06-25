package http

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
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

	h.serveError(rec, req, redirectErr)

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

func (e *testRedirectErr) Error() string           { return "redirect" }
func (e *testRedirectErr) RedirectURL() string     { return e.url }
func (e *testRedirectErr) RedirectStatusCode() int { return e.code }

var _ core.RedirectError = (*testRedirectErr)(nil)
var _ error = (*testRedirectErr)(nil)
