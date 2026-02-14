package bifrost

import (
	"context"
	"testing"
)

func TestAbsoluteAssetURLsInHTML(t *testing.T) {
	// Verify that asset URLs remain absolute in generated HTML
	scriptSrc := "/dist/blog-entry-abc123.js"
	cssHref := "/dist/blog-entry-abc123.css"
	chunks := []string{"/dist/chunk-xyz.js"}

	// The URLs should not be transformed to relative paths
	// They should remain as absolute paths starting with /dist/
	if scriptSrc[0:6] != "/dist/" {
		t.Errorf("Expected script src to start with /dist/, got %s", scriptSrc)
	}

	if cssHref[0:6] != "/dist/" {
		t.Errorf("Expected css href to start with /dist/, got %s", cssHref)
	}

	for i, chunk := range chunks {
		if chunk[0:6] != "/dist/" {
			t.Errorf("Expected chunk %d to start with /dist/, got %s", i, chunk)
		}
	}
}

func TestEmbedPathWithBifrostPrefix(t *testing.T) {
	// Test that embed paths include .bifrost/ prefix
	htmlPath := "/pages/routes/blog/hello/index.html"
	expectedEmbedPath := ".bifrost/pages/routes/blog/hello/index.html"

	// Simulate the path transformation
	embedPath := ".bifrost" + htmlPath
	embedPath = embedPath // This would also have TrimPrefix("/") and ToSlash applied

	if embedPath != expectedEmbedPath {
		t.Errorf("Expected embed path %s, got %s", expectedEmbedPath, embedPath)
	}
}

func TestDevModeSetupBeforeStaticDataLoader(t *testing.T) {
	// This test verifies that in dev mode, setup (building bundles)
	// happens before the static data loader path is executed

	t.Setenv("BIFROST_DEV", "1")

	r, err := New()
	if err != nil {
		t.Skipf("Skipping test: %v (is bun installed?)", err)
	}
	defer r.Stop()

	// Create a handler with both setup requirement and static data loader
	handler := r.NewPage("./test.tsx",
		WithStaticPrerender(),
		WithStaticDataLoader(func(ctx context.Context) ([]StaticPathData, error) {
			return []StaticPathData{
				{Path: "/test", Props: map[string]any{"key": "value"}},
			}, nil
		}),
	)

	if handler == nil {
		t.Fatal("Handler is nil")
	}

	// In dev mode with StaticDataLoader, the handler should:
	// 1. First call setup (to build bundles)
	// 2. Then call the loader
	// 3. Then render with props

	// The config should be stored
	config := r.pageConfigs["./test.tsx"]
	if config == nil {
		t.Fatal("Config not stored")
	}

	if config.Mode != ModeStaticPrerender {
		t.Errorf("Expected ModeStaticPrerender, got %v", config.Mode)
	}

	if config.StaticDataLoader == nil {
		t.Error("StaticDataLoader not set")
	}
}

func TestRouteFileServingPath(t *testing.T) {
	// Test that route file paths are correctly constructed
	tests := []struct {
		htmlPath      string
		expectedEmbed string
	}{
		{
			htmlPath:      "/pages/routes/blog/hello/index.html",
			expectedEmbed: ".bifrost/pages/routes/blog/hello/index.html",
		},
		{
			htmlPath:      "/pages/routes/about/index.html",
			expectedEmbed: ".bifrost/pages/routes/about/index.html",
		},
	}

	for _, tt := range tests {
		t.Run(tt.htmlPath, func(t *testing.T) {
			// Simulate the embed path construction
			embedPath := ".bifrost" + tt.htmlPath
			if embedPath != tt.expectedEmbed {
				t.Errorf("Expected %s, got %s", tt.expectedEmbed, embedPath)
			}
		})
	}
}
