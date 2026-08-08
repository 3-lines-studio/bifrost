package core

import (
	"context"
	"net/http"
	"testing"
)

func mustPageConfig(t *testing.T, route Route) PageConfig {
	t.Helper()
	config, err := PageConfigFromRoute(route)
	if err != nil {
		t.Fatalf("PageConfigFromRoute() error: %v", err)
	}
	return config
}

func TestPageCreatesRoute(t *testing.T) {
	route := Page("/", "./pages/home.tsx", WithLoader(func(*http.Request) (any, error) {
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

func TestPageConfigFromRoute_RejectsInvalidRoute(t *testing.T) {
	tests := []struct {
		name  string
		route Route
	}{
		{name: "conflicting modes", route: Page("/", "./page.tsx", WithClient(), WithStatic())},
		{name: "empty pattern", route: Page("", "./page.tsx")},
		{name: "empty component", route: Page("/", "")},
		{name: "blank component", route: Page("/", "  ")},
		{name: "nil option", route: Page("/", "./page.tsx", nil)},
		{name: "nil props loader", route: Page("/", "./page.tsx", WithLoader(nil))},
		{name: "nil pre loader", route: Page("/", "./page.tsx", WithPreLoader(nil))},
		{name: "nil static data loader", route: Page("/", "./page.tsx", WithStaticData(nil))},
		{name: "loader on client page", route: Page("/", "./page.tsx", WithClient(), WithLoader(func(*http.Request) (any, error) { return nil, nil }))},
		{name: "pre loader on client page", route: Page("/", "./page.tsx", WithClient(), WithPreLoader(func(*http.Request) (PreLoaderResult, error) { return PreLoaderResult{}, nil }))},
		{name: "loader on static page", route: Page("/", "./page.tsx", WithStatic(), WithLoader(func(*http.Request) (any, error) { return nil, nil }))},
		{name: "redundant static options", route: Page("/", "./page.tsx", WithStatic(), WithStaticData(func(context.Context) ([]StaticPathData, error) { return nil, nil }))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := PageConfigFromRoute(tt.route); err == nil {
				t.Fatal("expected invalid route to return an error")
			}
		})
	}
}

func TestPageOptions(t *testing.T) {
	t.Run("WithLoader creates route with loader", func(t *testing.T) {
		route := Page("/test", "./test.tsx", WithLoader(func(*http.Request) (any, error) {
			return map[string]any{"test": "value"}, nil
		}))

		if route.Pattern != "/test" {
			t.Errorf("Expected pattern '/test', got '%s'", route.Pattern)
		}

		if len(route.Options) != 1 {
			t.Errorf("Expected 1 option, got %d", len(route.Options))
		}
	})

	t.Run("WithPreLoader creates route with pre loader", func(t *testing.T) {
		route := Page("/test", "./test.tsx", WithPreLoader(func(*http.Request) (PreLoaderResult, error) {
			return PreLoaderResult{Lang: "pt"}, nil
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
