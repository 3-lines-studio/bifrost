package usecase

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestExportStaticPages_MergesRoutesForSharedComponent(t *testing.T) {
	const component = "./pages/shared.tsx"
	entryName := core.EntryNameForPath(component)
	outputDir := t.TempDir()
	routes := []core.Route{
		core.Page("/first", component, core.WithStatic()),
		core.Page("/second", component, core.WithStatic()),
	}
	renderer := &fakeRenderer{renderFn: func(string, any) (core.RenderedPage, error) {
		return core.RenderedPage{Body: "<p>page</p>"}, nil
	}}

	err := ExportStaticPages(ExportStaticPagesInput{
		OutputDir: outputDir,
		Routes:    routes,
		Manifest: &core.Manifest{Entries: map[string]core.ManifestEntry{
			entryName: {Script: "/dist/shared.js"},
		}},
		SSBundlePath: func(string) string { return "/tmp/shared-ssr.js" },
		Renderer:     renderer,
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(outputDir, "export-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest core.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	entry := manifest.Entries[entryName]
	if len(entry.StaticRoutes) != 2 {
		t.Fatalf("static routes = %#v, want 2 routes", entry.StaticRoutes)
	}
	if _, ok := entry.StaticRoutes["/first"]; !ok {
		t.Fatal("missing /first")
	}
	if _, ok := entry.StaticRoutes["/second"]; !ok {
		t.Fatal("missing /second")
	}
}
