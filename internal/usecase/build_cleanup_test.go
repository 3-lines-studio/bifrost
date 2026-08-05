package usecase

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestRemoveStaticSSRBundlesKeepsRuntimeSSRPages(t *testing.T) {
	root := t.TempDir()
	ssrDir := filepath.Join(root, "ssr")
	if err := os.MkdirAll(ssrDir, 0o755); err != nil {
		t.Fatal(err)
	}
	staticBundle := filepath.Join(ssrDir, "static-entry-ssr.js")
	ssrBundle := filepath.Join(ssrDir, "live-entry-ssr.js")
	for _, name := range []string{staticBundle, ssrBundle} {
		if err := os.WriteFile(name, []byte("bundle"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run := &buildRun{
		paths: buildPaths{bifrostDir: root, ssrDir: ssrDir},
		pages: []buildPage{
			{entryName: "static-entry", config: core.PageConfig{Mode: core.ModeStaticPrerender}},
			{entryName: "live-entry", config: core.PageConfig{Mode: core.ModeSSR}},
		},
		manifest: &core.Manifest{Entries: map[string]core.ManifestEntry{
			"static-entry": {Mode: "static", SSR: "/ssr/static-entry-ssr.js"},
			"live-entry":   {Mode: "ssr", SSR: "/ssr/live-entry-ssr.js"},
		}},
		needsRuntime: true,
	}

	if err := removeStaticSSRBundles(run); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(staticBundle); !os.IsNotExist(err) {
		t.Fatalf("static SSR bundle still exists: %v", err)
	}
	if _, err := os.Stat(ssrBundle); err != nil {
		t.Fatalf("runtime SSR bundle was removed: %v", err)
	}
	if got := run.manifest.Entries["static-entry"].SSR; got != "" {
		t.Fatalf("static manifest SSR path = %q, want empty", got)
	}
}

func TestRemoveStaticSSRBundlesRemovesStaticOnlyDirectory(t *testing.T) {
	root := t.TempDir()
	ssrDir := filepath.Join(root, "ssr")
	if err := os.MkdirAll(filepath.Join(ssrDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ssrDir, "nested", "artifact.css"), []byte("css"), 0o644); err != nil {
		t.Fatal(err)
	}

	run := &buildRun{
		paths: buildPaths{bifrostDir: root, ssrDir: ssrDir},
		pages: []buildPage{{entryName: "static-entry", config: core.PageConfig{Mode: core.ModeStaticPrerender}}},
		manifest: &core.Manifest{Entries: map[string]core.ManifestEntry{
			"static-entry": {Mode: "static"},
		}},
	}
	if err := removeStaticSSRBundles(run); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ssrDir); !os.IsNotExist(err) {
		t.Fatalf("static-only SSR directory still exists: %v", err)
	}
}
