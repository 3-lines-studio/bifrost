package bifrost

import (
	"fmt"
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
	// Dev mode allows rendering raw TSX files
	t.Setenv("BIFROST_DEV", "1")

	r, err := New()
	if err != nil {
		t.Skipf("Skipping test: %v (is bun installed?)", err)
	}
	defer r.Stop()

	page, err := r.Render("./example/components/hello.tsx", map[string]any{
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
	// Dev mode allows rendering raw TSX files
	t.Setenv("BIFROST_DEV", "1")

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
