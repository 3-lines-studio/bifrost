package bifrost

import (
	"context"
	"testing"
)

func TestWithStaticDataLoader(t *testing.T) {
	t.Setenv("BIFROST_DEV", "1")

	r, err := New()
	if err != nil {
		t.Skipf("Skipping test: %v (is bun installed?)", err)
	}
	defer r.Stop()

	loader := func(ctx context.Context) ([]StaticPathData, error) {
		return []StaticPathData{
			{Path: "/test", Props: map[string]any{"key": "value"}},
		}, nil
	}

	handler := r.NewPage("./test.tsx",
		WithStaticPrerender(),
		WithStaticDataLoader(loader),
	)
	if handler == nil {
		t.Error("NewPage returned nil handler")
	}

	// Verify loader is stored in config
	config := r.pageConfigs["./test.tsx"]
	if config == nil {
		t.Fatal("Config not stored")
	}
	if config.StaticDataLoader == nil {
		t.Error("StaticDataLoader not set in config")
	}
	if config.Mode != ModeStaticPrerender {
		t.Errorf("Expected ModeStaticPrerender, got %v", config.Mode)
	}
}

func TestExportStaticBuildData(t *testing.T) {
	t.Setenv("BIFROST_EXPORT_STATIC", "1")

	r, err := New()
	if err != nil {
		t.Fatalf("Failed to create renderer: %v", err)
	}

	loaderCalled := false
	loader := func(ctx context.Context) ([]StaticPathData, error) {
		loaderCalled = true
		return []StaticPathData{
			{Path: "/blog/hello", Props: map[string]any{"slug": "hello"}},
			{Path: "/blog/world", Props: map[string]any{"slug": "world"}},
		}, nil
	}

	r.NewPage("./pages/blog.tsx",
		WithStaticPrerender(),
		WithStaticDataLoader(loader),
	)

	handled, err := ExportStaticBuildData(r)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if !handled {
		t.Error("Expected handled to be true")
	}

	if !loaderCalled {
		t.Error("Loader was not called")
	}
}

func TestExportStaticBuildDataNotInExportMode(t *testing.T) {
	t.Setenv("BIFROST_EXPORT_STATIC", "")
	t.Setenv("BIFROST_DEV", "1")

	r, err := New()
	if err != nil {
		t.Skipf("Skipping test: %v (is bun installed?)", err)
	}
	defer r.Stop()

	handled, err := ExportStaticBuildData(r)
	if err != nil {
		t.Fatalf("Export check failed: %v", err)
	}
	if handled {
		t.Error("Expected handled to be false when not in export mode")
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
	export := StaticBuildExport{
		Version: 1,
		Pages: []StaticPageExport{
			{
				ComponentPath: "./pages/blog.tsx",
				Entries: []StaticPathExport{
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
