package bifrost

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// mockRedirectError implements RedirectError for testing
type mockRedirectError struct {
	url    string
	status int
}

func (m *mockRedirectError) Error() string {
	return fmt.Sprintf("redirect to %s", m.url)
}

func (m *mockRedirectError) RedirectURL() string {
	return m.url
}

func (m *mockRedirectError) RedirectStatusCode() int {
	return m.status
}

func TestRenderer(t *testing.T) {
	r, err := New()
	if err != nil {
		t.Skipf("Skipping test: %v (is bun installed?)", err)
	}
	defer r.Stop()

	page, err := r.Render("./example/components/Hello.tsx", map[string]interface{}{
		"name": "World",
	})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if !strings.Contains(page.Body, "Hello,") || !strings.Contains(page.Body, "World") {
		t.Errorf("Expected 'Hello, World' content in output, got: %s", page.Body)
	}
}

func TestRendererMissingPath(t *testing.T) {
	r, err := New()
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}
	defer r.Stop()

	_, err = r.Render("./example/components/NonExistent.tsx", nil)
	if err == nil {
		t.Error("Expected error for non-existent component")
	}
}

func TestRedirectError(t *testing.T) {
	tests := []struct {
		name         string
		redirectErr  *mockRedirectError
		expectedURL  string
		expectedCode int
	}{
		{
			name:         "custom 301 redirect",
			redirectErr:  &mockRedirectError{url: "/new-path", status: http.StatusMovedPermanently},
			expectedURL:  "/new-path",
			expectedCode: http.StatusMovedPermanently,
		},
		{
			name:         "custom 302 redirect",
			redirectErr:  &mockRedirectError{url: "/temp-path", status: http.StatusFound},
			expectedURL:  "/temp-path",
			expectedCode: http.StatusFound,
		},
		{
			name:         "zero status defaults to 302",
			redirectErr:  &mockRedirectError{url: "/default-path", status: 0},
			expectedURL:  "/default-path",
			expectedCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.redirectErr.RedirectURL() != tt.expectedURL {
				t.Errorf("RedirectURL() = %q, want %q", tt.redirectErr.RedirectURL(), tt.expectedURL)
			}
			if tt.redirectErr.RedirectStatusCode() != tt.expectedCode {
				t.Errorf("RedirectStatusCode() = %d, want %d", tt.redirectErr.RedirectStatusCode(), tt.expectedCode)
			}
		})
	}
}
