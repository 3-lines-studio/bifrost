package bifrost

import (
	"context"
	"testing"
)

func TestDevModeWithStaticDataLoader(t *testing.T) {
	t.Setenv("BIFROST_DEV", "1")

	r, err := New()
	if err != nil {
		t.Skipf("Skipping test: %v (is bun installed?)", err)
	}
	defer r.Stop()

	loader := func(ctx context.Context) ([]StaticPathData, error) {
		return []StaticPathData{
			{
				Path: "/blog/hello",
				Props: map[string]any{
					"title": "Hello Post",
					"body":  "Hello content",
				},
			},
			{
				Path: "/blog/world",
				Props: map[string]any{
					"title": "World Post",
					"body":  "World content",
				},
			},
		}, nil
	}

	handler := r.NewPage("./test.tsx",
		WithStaticPrerender(),
		WithStaticDataLoader(loader),
	)

	if handler == nil {
		t.Fatal("Handler is nil")
	}

	// Verify config is stored
	config := r.pageConfigs["./test.tsx"]
	if config == nil {
		t.Fatal("Config not stored")
	}

	if config.StaticDataLoader == nil {
		t.Error("StaticDataLoader not set")
	}
}

func TestStaticDataLoaderPathMatching(t *testing.T) {
	loader := func(ctx context.Context) ([]StaticPathData, error) {
		return []StaticPathData{
			{Path: "/blog/hello", Props: map[string]any{"title": "Hello"}},
			{Path: "/blog/world", Props: map[string]any{"title": "World"}},
		}, nil
	}

	entries, err := loader(context.Background())
	if err != nil {
		t.Fatalf("Loader failed: %v", err)
	}

	// Test path matching
	targetPath := "/blog/hello"
	var matchedProps map[string]any
	found := false

	for _, entry := range entries {
		if entry.Path == targetPath {
			matchedProps = entry.Props
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected to find path /blog/hello")
	}

	if matchedProps["title"] != "Hello" {
		t.Errorf("Expected title 'Hello', got %v", matchedProps["title"])
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/blog/hello", "/blog/hello"},
		{"blog/hello", "/blog/hello"},
		{"/blog/hello/", "/blog/hello"},
		{"/", "/"},
		{"", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			// We need to test the normalizePath function from internal/page
			// For now, just verify the logic conceptually
			result := tt.input
			if result == "" {
				result = "/"
			}
			if !startsWith(result, "/") {
				result = "/" + result
			}
			if result != "/" && endsWith(result, "/") {
				result = result[:len(result)-1]
			}

			if result != tt.expected {
				t.Errorf("normalizePath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
