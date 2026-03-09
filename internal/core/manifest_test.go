package core

import (
	"encoding/json"
	"testing"
)

func TestHasSSREntries_ModeSSR(t *testing.T) {
	man := &Manifest{
		Entries: map[string]ManifestEntry{
			"pages-home-entry": {
				Script: "/dist/pages-home-entry.js",
				Mode:   "ssr",
				SSR:    "/ssr/pages-home-entry-ssr.js",
			},
		},
	}
	if !HasSSREntries(man) {
		t.Error("expected HasSSREntries=true for mode=ssr")
	}
}

func TestHasSSREntries_StaticWithSSRBundle(t *testing.T) {
	man := &Manifest{
		Entries: map[string]ManifestEntry{
			"pages-product-entry": {
				Script: "/dist/pages-product-entry.js",
				Mode:   "static",
				SSR:    "/ssr/pages-product-entry-ssr.js",
			},
		},
	}
	if HasSSREntries(man) {
		t.Error("expected HasSSREntries=false for static page (no runtime needed)")
	}
}

func TestHasSSRBundles_StaticWithSSRBundle(t *testing.T) {
	man := &Manifest{
		Entries: map[string]ManifestEntry{
			"pages-product-entry": {
				Script: "/dist/pages-product-entry.js",
				Mode:   "static",
				SSR:    "/ssr/pages-product-entry-ssr.js",
			},
		},
	}
	if !HasSSRBundles(man) {
		t.Error("expected HasSSRBundles=true for static page with SSR bundle")
	}
}

func TestHasSSREntries_ClientOnlyNoSSR(t *testing.T) {
	man := &Manifest{
		Entries: map[string]ManifestEntry{
			"pages-about-entry": {
				Script: "/dist/pages-about-entry.js",
				Mode:   "client",
				SSR:    "",
			},
		},
	}
	if HasSSREntries(man) {
		t.Error("expected HasSSREntries=false for client-only page without SSR bundle")
	}
}

func TestHasSSREntries_NilManifest(t *testing.T) {
	if HasSSREntries(nil) {
		t.Error("expected HasSSREntries=false for nil manifest")
	}
}

func TestHasSSREntries_EmptyEntries(t *testing.T) {
	man := &Manifest{
		Entries: map[string]ManifestEntry{},
	}
	if HasSSREntries(man) {
		t.Error("expected HasSSREntries=false for empty entries")
	}
}

func TestHasSSRBundles_NilManifest(t *testing.T) {
	if HasSSRBundles(nil) {
		t.Error("expected HasSSRBundles=false for nil manifest")
	}
}

func TestHasSSRBundles_EmptyEntries(t *testing.T) {
	man := &Manifest{
		Entries: map[string]ManifestEntry{},
	}
	if HasSSRBundles(man) {
		t.Error("expected HasSSRBundles=false for empty entries")
	}
}

func TestHasSSRBundles_ClientOnlyNoSSR(t *testing.T) {
	man := &Manifest{
		Entries: map[string]ManifestEntry{
			"pages-about-entry": {
				Script: "/dist/pages-about-entry.js",
				Mode:   "client",
				SSR:    "",
			},
		},
	}
	if HasSSRBundles(man) {
		t.Error("expected HasSSRBundles=false for client-only page without SSR bundle")
	}
}

func TestHasSSREntries_MixedModes(t *testing.T) {
	man := &Manifest{
		Entries: map[string]ManifestEntry{
			"client-page": {
				Script: "/dist/client.js",
				Mode:   "client",
			},
			"static-page": {
				Script: "/dist/static.js",
				Mode:   "static",
				SSR:    "/ssr/static-ssr.js",
			},
		},
	}
	if HasSSREntries(man) {
		t.Error("expected HasSSREntries=false when no SSR mode pages exist")
	}
}

func TestHasSSRBundles_MixedModes(t *testing.T) {
	man := &Manifest{
		Entries: map[string]ManifestEntry{
			"client-page": {
				Script: "/dist/client.js",
				Mode:   "client",
			},
			"static-page": {
				Script: "/dist/static.js",
				Mode:   "static",
				SSR:    "/ssr/static-ssr.js",
			},
		},
	}
	if !HasSSRBundles(man) {
		t.Error("expected HasSSRBundles=true when any entry has SSR bundle")
	}
}

func TestParseManifest(t *testing.T) {
	raw := `{
		"entries": {
			"pages-home-entry": {
				"script": "/dist/pages-home-entry.js",
				"css": "/dist/pages-home-entry.css",
				"ssr": "/ssr/pages-home-entry-ssr.js",
				"mode": "ssr"
			}
		}
	}`

	man, err := ParseManifest([]byte(raw))
	if err != nil {
		t.Fatalf("ParseManifest failed: %v", err)
	}

	entry, ok := man.Entries["pages-home-entry"]
	if !ok {
		t.Fatal("expected pages-home-entry in manifest")
	}
	if entry.Script != "/dist/pages-home-entry.js" {
		t.Errorf("unexpected script: %s", entry.Script)
	}
	if entry.SSR != "/ssr/pages-home-entry-ssr.js" {
		t.Errorf("unexpected ssr: %s", entry.SSR)
	}
}

func TestParseManifest_Invalid(t *testing.T) {
	_, err := ParseManifest([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestGetAssets_WithManifest(t *testing.T) {
	man := &Manifest{
		Entries: map[string]ManifestEntry{
			"pages-home-entry": {
				Script: "/dist/pages-home-entry-abc123.js",
				CSS:    "/dist/pages-home-entry-abc123.css",
				Chunks: []string{"/dist/chunk-xyz.js"},
				SSR:    "/ssr/pages-home-entry-ssr.js",
			},
		},
	}

	assets := GetAssets(man, "pages-home-entry")
	if assets.Script != "/dist/pages-home-entry-abc123.js" {
		t.Errorf("unexpected script: %s", assets.Script)
	}
	if assets.CSS != "/dist/pages-home-entry-abc123.css" {
		t.Errorf("unexpected css: %s", assets.CSS)
	}
	if len(assets.Chunks) != 1 || assets.Chunks[0] != "/dist/chunk-xyz.js" {
		t.Errorf("unexpected chunks: %v", assets.Chunks)
	}
}

func TestGetAssets_FallbackWithoutManifest(t *testing.T) {
	assets := GetAssets(nil, "pages-home-entry")
	if assets.Script != "/dist/pages-home-entry.js" {
		t.Errorf("unexpected fallback script: %s", assets.Script)
	}
	if assets.CSS != "/dist/pages-home-entry.css" {
		t.Errorf("unexpected fallback css: %s", assets.CSS)
	}
}

func TestManifestEntryJSON_StaticRoutes(t *testing.T) {
	entry := ManifestEntry{
		Script: "/dist/pages-blog-entry.js",
		CSS:    "/dist/pages-blog-entry.css",
		Mode:   "static",
		StaticRoutes: map[string]string{
			"/blog/hello": "/pages/routes/blog/hello/index.html",
		},
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed ManifestEntry
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if parsed.StaticRoutes["/blog/hello"] != "/pages/routes/blog/hello/index.html" {
		t.Errorf("unexpected static route: %s", parsed.StaticRoutes["/blog/hello"])
	}
}
