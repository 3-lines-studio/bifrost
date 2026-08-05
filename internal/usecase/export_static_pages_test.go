package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestExportStaticPagesReturnsRenderErrors(t *testing.T) {
	const component = "./pages/broken.tsx"
	err := ExportStaticPages(ExportStaticPagesInput{
		OutputDir: t.TempDir(),
		Routes:    []core.Route{core.Page("/broken", component, core.WithStatic())},
		Manifest: &core.Manifest{Entries: map[string]core.ManifestEntry{
			core.EntryNameForPath(component): {Script: "/dist/broken.js"},
		}},
		SSBundlePath: func(string) string { return "/tmp/broken-ssr.js" },
		Renderer: &fakeRenderer{renderFn: func(string, any) (core.RenderedPage, error) {
			return core.RenderedPage{}, errors.New("render failed")
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "render failed") {
		t.Fatalf("ExportStaticPages() error = %v, want render failure", err)
	}
}

func TestExportStaticPagesReturnsLoaderErrors(t *testing.T) {
	const component = "./pages/broken.tsx"
	err := ExportStaticPages(ExportStaticPagesInput{
		OutputDir: t.TempDir(),
		Routes: []core.Route{core.Page("/broken/{slug}", component, core.WithStaticData(func(context.Context) ([]core.StaticPathData, error) {
			return nil, errors.New("load failed")
		}))},
		Manifest: &core.Manifest{Entries: map[string]core.ManifestEntry{
			core.EntryNameForPath(component): {Script: "/dist/broken.js"},
		}},
		SSBundlePath: func(string) string { return "/tmp/broken-ssr.js" },
		Renderer:     &fakeRenderer{},
	})
	if err == nil || !strings.Contains(err.Error(), "load failed") {
		t.Fatalf("ExportStaticPages() error = %v, want loader failure", err)
	}
}

func TestNormalizeStaticExportPathRejectsUnsafeOrDynamicPaths(t *testing.T) {
	for _, raw := range []string{"", "../outside", "/a/../outside", `\\outside`, "/page?x=1", "/page#part", "/blog/{slug}"} {
		t.Run(raw, func(t *testing.T) {
			if _, _, err := normalizeStaticExportPath(raw); err == nil {
				t.Fatalf("normalizeStaticExportPath(%q) returned no error", raw)
			}
		})
	}
}

func TestNormalizeStaticExportPathNormalizesURLPaths(t *testing.T) {
	normalized, cleaned, err := normalizeStaticExportPath("blog//post/")
	if err != nil {
		t.Fatal(err)
	}
	if normalized != "/blog/post" || cleaned != "/blog/post" {
		t.Fatalf("got normalized=%q cleaned=%q", normalized, cleaned)
	}
}
