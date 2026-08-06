package modernc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sobekrenderer "github.com/3-lines-studio/bifrost/internal/adapters/sobek"
	"github.com/3-lines-studio/bifrost/internal/core"
)

// TestParityWithSobekOnExamplePage renders the example home page with both
// in-process runtimes and requires byte-identical output. It skips when the
// example artifacts are unavailable or are an SSR registry build (which the
// modernc runtime does not produce).
func TestParityWithSobekOnExamplePage(t *testing.T) {
	bundle, skip := exampleHomeBundle(t)
	if skip {
		t.Skip("example SSR bundle unavailable or a registry build")
	}
	props := map[string]any{"name": "Parity"}

	expectedRenderer, err := sobekrenderer.NewRenderer(core.ModeProd, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = expectedRenderer.Stop() }()
	expected, err := expectedRenderer.Render(bundle, props)
	if err != nil {
		t.Fatal(err)
	}

	renderer, err := NewRenderer(core.ModeProd, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = renderer.Stop() }()
	got, err := renderer.Render(bundle, props)
	if err != nil {
		t.Fatal(err)
	}

	if got.Head != expected.Head {
		t.Errorf("head mismatch: %d bytes vs sobek %d bytes", len(got.Head), len(expected.Head))
	}
	if got.Body != expected.Body {
		t.Errorf("body mismatch: %d bytes vs sobek %d bytes", len(got.Body), len(expected.Body))
	}
}

func exampleHomeBundle(tb testing.TB) (string, bool) {
	tb.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		tb.Fatal(err)
	}
	bifrostDir := filepath.Join(repoRoot, "example", "cmd", "full", ".bifrost")
	manifestData, err := os.ReadFile(filepath.Join(bifrostDir, "manifest.json"))
	if err != nil {
		return "", true
	}
	var manifest core.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return "", true
	}
	entry, ok := manifest.Entries["pages-home-entry"]
	if !ok || entry.SSR == "" {
		return "", true
	}
	if strings.Contains(entry.SSR, "#") {
		return "", true
	}
	parts := strings.SplitN(entry.SSR, "#", 2)
	bundlePath := filepath.Join(bifrostDir, filepath.FromSlash(strings.TrimPrefix(parts[0], "/")))
	if _, err := os.Stat(bundlePath); err != nil {
		return "", true
	}
	return bundlePath, false
}
