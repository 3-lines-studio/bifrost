package bifrost

import (
	"context"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/types"
)

func TestWithStaticData(t *testing.T) {
	t.Setenv("BIFROST_DEV", "1")

	loader := func(ctx context.Context) ([]StaticPathData, error) {
		return []StaticPathData{
			{Path: "/test", Props: map[string]any{"key": "value"}},
		}, nil
	}

	route := Page("/blog", "./blog.tsx", WithStaticData(loader))

	if route.Pattern != "/blog" {
		t.Errorf("Expected pattern '/blog', got '%s'", route.Pattern)
	}

	if len(route.Options) != 1 {
		t.Errorf("Expected 1 option, got %d", len(route.Options))
	}

	app := New(testFS, route)
	defer app.Stop()

	config := app.pageConfigs["./blog.tsx"]
	if config == nil {
		t.Fatal("Config not stored")
	}
	if config.StaticDataLoader == nil {
		t.Error("StaticDataLoader not set in config")
	}
	if config.Mode != types.ModeStaticPrerender {
		t.Errorf("Expected ModeStaticPrerender, got %v", config.Mode)
	}
}

func TestStaticPathDataStructure(t *testing.T) {
	data := StaticPathData{
		Path: "/blog/test",
		Props: map[string]any{
			"title": "Test Post",
			"slug":  "test",
		},
	}

	if data.Path != "/blog/test" {
		t.Errorf("Expected Path to be /blog/test, got %s", data.Path)
	}

	if data.Props["title"] != "Test Post" {
		t.Errorf("Expected title prop, got %v", data.Props["title"])
	}
}

func TestStaticBuildExportStructure(t *testing.T) {
	export := staticBuildExport{
		Version: 1,
		Pages: []staticPageExport{
			{
				ComponentPath: "./pages/blog.tsx",
				Entries: []staticPathExport{
					{Path: "/blog/hello", Props: map[string]any{"slug": "hello"}},
					{Path: "/blog/world", Props: map[string]any{"slug": "world"}},
				},
			},
		},
	}

	if export.Version != 1 {
		t.Errorf("Expected Version 1, got %d", export.Version)
	}

	if len(export.Pages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(export.Pages))
	}

	page := export.Pages[0]
	if page.ComponentPath != "./pages/blog.tsx" {
		t.Errorf("Expected ComponentPath ./pages/blog.tsx, got %s", page.ComponentPath)
	}

	if len(page.Entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(page.Entries))
	}
}
