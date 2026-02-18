package bifrost

import (
	"context"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/types"
)

func TestAbsoluteAssetURLsInHTML(t *testing.T) {
	scriptSrc := "/dist/blog-entry-abc123.js"
	cssHref := "/dist/blog-entry-abc123.css"
	chunks := []string{"/dist/chunk-xyz.js"}

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
	htmlPath := "/pages/routes/blog/hello/index.html"
	expectedEmbedPath := ".bifrost/pages/routes/blog/hello/index.html"

	embedPath := ".bifrost" + htmlPath

	if embedPath != expectedEmbedPath {
		t.Errorf("Expected embed path %s, got %s", expectedEmbedPath, embedPath)
	}
}

func TestDevModeSetupBeforeStaticDataLoader(t *testing.T) {
	t.Setenv("BIFROST_DEV", "1")

	loader := func(ctx context.Context) ([]StaticPathData, error) {
		return []StaticPathData{
			{Path: "/test", Props: map[string]any{"key": "value"}},
		}, nil
	}

	route := Page("/blog", "./blog.tsx", WithStaticData(loader))

	app := New(testFS, route)
	defer app.Stop()

	config := app.pageConfigs["./blog.tsx"]
	if config == nil {
		t.Fatal("Config not stored")
	}

	if config.Mode != types.ModeStaticPrerender {
		t.Errorf("Expected ModeStaticPrerender, got %v", config.Mode)
	}

	if config.StaticDataLoader == nil {
		t.Error("StaticDataLoader not set")
	}
}

func TestRouteFileServingPath(t *testing.T) {
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
			embedPath := ".bifrost" + tt.htmlPath
			if embedPath != tt.expectedEmbed {
				t.Errorf("Expected %s, got %s", tt.expectedEmbed, embedPath)
			}
		})
	}
}
