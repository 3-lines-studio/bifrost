package usecase

import "testing"

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
