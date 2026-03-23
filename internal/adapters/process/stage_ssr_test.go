package process

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/adapters/process/testembed"
	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestStageSSRBundles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := []byte("// ssr bundle")
	if err := os.MkdirAll(filepath.Join(dir, "ssr"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "ssr", "pages-home-entry-ssr.js")
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}

	man := &core.Manifest{
		Entries: map[string]core.ManifestEntry{
			"pages-home-entry": {SSR: "/ssr/pages-home-entry-ssr.js"},
		},
	}

	read := func(manifestSSRPath string) ([]byte, error) {
		clean := filepath.ToSlash(manifestSSRPath)
		if len(clean) > 0 && clean[0] == '/' {
			clean = clean[1:]
		}
		return os.ReadFile(filepath.Join(dir, filepath.FromSlash(clean)))
	}

	tempDir, cleanup, err := StageSSRBundles(read, man)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	out := filepath.Join(tempDir, "ssr", "pages-home-entry-ssr.js")
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("content mismatch")
	}
}

func TestExtractSSRBundlesFromEmbed(t *testing.T) {
	t.Parallel()

	manifestData, err := testembed.Assets.ReadFile(".bifrost/manifest.json")
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifest, err := core.ParseManifest(manifestData)
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}

	tempDir, cleanup, err := ExtractSSRBundles(testembed.Assets, manifest)
	if err != nil {
		t.Fatalf("extract SSR bundles: %v", err)
	}
	defer cleanup()

	got, err := os.ReadFile(filepath.Join(tempDir, "ssr", "pages-home-entry-ssr.js"))
	if err != nil {
		t.Fatalf("read staged bundle: %v", err)
	}
	if strings.TrimSpace(string(got)) != `export default function render() { return "ok"; }` {
		t.Fatalf("unexpected staged content: %q", string(got))
	}
}

func TestResolveStagedSSRBundlePath(t *testing.T) {
	t.Parallel()

	got := ResolveStagedSSRBundlePath("/tmp/bifrost-ssr", "/ssr/pages-home-entry-ssr.js")
	want := filepath.Join("/tmp/bifrost-ssr", "ssr", "pages-home-entry-ssr.js")
	if got != want {
		t.Fatalf("ResolveStagedSSRBundlePath() = %q, want %q", got, want)
	}
}
