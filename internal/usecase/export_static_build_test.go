package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestWriteStaticBuildExport_MergesRoutesForSharedComponent(t *testing.T) {
	routes := []core.Route{
		core.Page("/first", "./pages/shared.tsx", core.WithStatic()),
		core.Page("/second/{slug}", "./pages/shared.tsx", core.WithStaticData(func(context.Context) ([]core.StaticPathData, error) {
			return []core.StaticPathData{{Path: "/second/a", Props: map[string]any{"slug": "a"}}}, nil
		})),
	}

	var buf bytes.Buffer
	if err := WriteStaticBuildExport(&buf, routes); err != nil {
		t.Fatal(err)
	}
	var got staticBuildExport
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Pages) != 1 {
		t.Fatalf("pages = %d, want 1", len(got.Pages))
	}
	if len(got.Pages[0].Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(got.Pages[0].Entries))
	}
	if got.Pages[0].Entries[0].Path != "/first" || got.Pages[0].Entries[1].Path != "/second/a" {
		t.Fatalf("unexpected entries: %#v", got.Pages[0].Entries)
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
