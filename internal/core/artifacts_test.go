package core

import "testing"

func TestResolvePageArtifacts_EquivalentToGetAssets(t *testing.T) {
	t.Parallel()
	man := &Manifest{
		Entries: map[string]ManifestEntry{
			"pages-home-entry": {
				Script:   "/dist/pages-home-entry-abc123.js",
				CSS:      "/dist/pages-home-entry-abc123.css",
				Chunks:   []string{"/dist/chunk-xyz.js"},
				SSR:      "/ssr/pages-home-entry-ssr.js",
				CriticalCSS: "body{color:red}",
			},
		},
	}
	entry := "pages-home-entry"
	a := ResolvePageArtifacts(man, entry)
	b := GetAssets(man, entry)
	if a.Script != b.Script || a.CSS != b.CSS || len(a.Chunks) != len(b.Chunks) {
		t.Fatalf("ResolvePageArtifacts vs GetAssets mismatch: %+v vs %+v", a, b)
	}
}

func TestResolvePageArtifacts_Fallback(t *testing.T) {
	t.Parallel()
	a := ResolvePageArtifacts(nil, "pages-home-entry")
	if a.Script != "/dist/pages-home-entry.js" || a.CSS != "/dist/pages-home-entry.css" {
		t.Fatalf("unexpected fallback: %+v", a)
	}
}

func TestStylesheetHrefsFor(t *testing.T) {
	t.Parallel()
	a := PageArtifacts{
		CSS:      "/dist/shared.css",
		CSSFiles: []string{"/dist/extra.css"},
	}
	h := StylesheetHrefsFor(a)
	if len(h) != 2 || h[0] != "/dist/shared.css" || h[1] != "/dist/extra.css" {
		t.Fatalf("got %v", h)
	}
}
