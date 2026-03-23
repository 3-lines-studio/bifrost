package core

import (
	"net/http"
	"testing"
)

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

func TestPageOptions(t *testing.T) {
	t.Run("WithLoader creates route with loader", func(t *testing.T) {
		route := Page("/test", "./test.tsx", WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{"test": "value"}, nil
		}))

		if route.Pattern != "/test" {
			t.Errorf("Expected pattern '/test', got '%s'", route.Pattern)
		}

		if len(route.Options) != 1 {
			t.Errorf("Expected 1 option, got %d", len(route.Options))
		}
	})

	t.Run("WithClient creates route with client option", func(t *testing.T) {
		route := Page("/about", "./about.tsx", WithClient())

		if route.Pattern != "/about" {
			t.Errorf("Expected pattern '/about', got '%s'", route.Pattern)
		}

		if len(route.Options) != 1 {
			t.Errorf("Expected 1 option, got %d", len(route.Options))
		}
	})

	t.Run("Page creates route without options", func(t *testing.T) {
		route := Page("/", "./home.tsx")

		if route.Pattern != "/" {
			t.Errorf("Expected pattern '/', got '%s'", route.Pattern)
		}

		if len(route.Options) != 0 {
			t.Errorf("Expected 0 options, got %d", len(route.Options))
		}
	})

	t.Run("WithStatic creates route with static option", func(t *testing.T) {
		route := Page("/blog", "./blog.tsx", WithStatic())

		if route.Pattern != "/blog" {
			t.Errorf("Expected pattern '/blog', got '%s'", route.Pattern)
		}

		if len(route.Options) != 1 {
			t.Errorf("Expected 1 option, got %d", len(route.Options))
		}
	})

}
