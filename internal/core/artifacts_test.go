package core

import "testing"

func TestResolvePageArtifactsFromManifest(t *testing.T) {
	t.Parallel()
	man := &Manifest{
		Entries: map[string]ManifestEntry{
			"pages-home-entry": {
				Script:      "/dist/pages-home-entry-abc123.js",
				CSS:         "/dist/pages-home-entry-abc123.css",
				Chunks:      []string{"/dist/chunk-xyz.js"},
				CriticalCSS: "body{color:red}",
			},
		},
	}
	a := ResolvePageArtifacts(man, "pages-home-entry")
	if a.Script != "/dist/pages-home-entry-abc123.js" || a.CSS != "/dist/pages-home-entry-abc123.css" || len(a.Chunks) != 1 {
		t.Fatalf("unexpected artifacts: %+v", a)
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
