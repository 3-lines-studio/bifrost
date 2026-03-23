package process

import (
	"os"
	"path/filepath"
	"testing"

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
