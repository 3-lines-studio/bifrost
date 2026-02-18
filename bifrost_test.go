package bifrost

import (
	"embed"
	"fmt"
	"net/http"
	"testing"
)

var testFS embed.FS

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

func TestNewCreatesApp(t *testing.T) {
	t.Setenv("BIFROST_DEV", "1")

	app := New(testFS)
	defer app.Stop()

	if app == nil {
		t.Error("New() returned nil app")
	}
}

func TestPageCreatesRoute(t *testing.T) {
	route := Page("/", "./pages/home.tsx", WithLoader(func(*http.Request) (map[string]any, error) {
		return map[string]any{"name": "World"}, nil
	}))

	if route.Pattern != "/" {
		t.Errorf("Expected pattern '/', got '%s'", route.Pattern)
	}

	if route.ComponentPath != "./pages/home.tsx" {
		t.Errorf("Expected component './pages/home.tsx', got '%s'", route.ComponentPath)
	}

	if len(route.Options) != 1 {
		t.Errorf("Expected 1 option, got %d", len(route.Options))
	}
}

func TestPageWithClient(t *testing.T) {
	route := Page("/about", "./pages/about.tsx", WithClient())

	if route.Pattern != "/about" {
		t.Errorf("Expected pattern '/about', got '%s'", route.Pattern)
	}

	if len(route.Options) != 1 {
		t.Errorf("Expected 1 option, got %d", len(route.Options))
	}
}

func TestPageWithStatic(t *testing.T) {
	route := Page("/blog", "./pages/blog.tsx", WithStatic())

	if route.Pattern != "/blog" {
		t.Errorf("Expected pattern '/blog', got '%s'", route.Pattern)
	}

	if len(route.Options) != 1 {
		t.Errorf("Expected 1 option, got %d", len(route.Options))
	}
}
