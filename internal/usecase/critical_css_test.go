package usecase

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestResolveBuiltAssetPath(t *testing.T) {
	got := resolveBuiltAssetPath("/tmp/.bifrost", "/dist/page.css")
	want := filepath.Join("/tmp/.bifrost", "dist", "page.css")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveBuiltAssetPath_RejectsTraversal(t *testing.T) {
	if got := resolveBuiltAssetPath("/tmp/.bifrost", "/../secret.css"); got != "" {
		t.Fatalf("expected empty path, got %q", got)
	}
}

func TestWriteClientOnlyHTML_IncludesCriticalAndStylesheet(t *testing.T) {
	svc := &BuildService{}
	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "page.html")

	err := svc.writeClientOnlyHTML(
		htmlPath,
		"Client Page",
		"/dist/page.js",
		".hero{display:block}",
		[]string{"/dist/page.css"},
		[]string{"/dist/chunk-a.js"},
		"en",
		"",
	)
	if err != nil {
		t.Fatalf("writeClientOnlyHTML failed: %v", err)
	}

	data, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("failed to read generated HTML: %v", err)
	}
	html := string(data)
	if !strings.Contains(html, `data-bifrost-critical`) {
		t.Fatal("expected inline critical CSS tag")
	}
	if !strings.Contains(html, `href="/dist/page.css"`) {
		t.Fatal("expected stylesheet link")
	}
	if !strings.Contains(html, `media="print"`) {
		t.Fatal("expected deferred non-critical stylesheet when critical CSS is present")
	}
}

func TestWriteClientOnlyHTML_MultipleStylesheets(t *testing.T) {
	svc := &BuildService{}
	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "page.html")

	err := svc.writeClientOnlyHTML(
		htmlPath,
		"Client Page",
		"/dist/page.js",
		"",
		[]string{"/dist/a.css", "/dist/b.css"},
		nil,
		"en",
		"",
	)
	if err != nil {
		t.Fatalf("writeClientOnlyHTML failed: %v", err)
	}
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	if !strings.Contains(html, `href="/dist/a.css"`) || !strings.Contains(html, `href="/dist/b.css"`) {
		t.Fatalf("expected both stylesheet links: %s", html)
	}
}

func TestPageServiceRenderPageHTML_IncludesCriticalAndStylesheet(t *testing.T) {
	svc := &PageService{}
	manifest := &core.Manifest{
		Entries: map[string]core.ManifestEntry{
			"home": {
				Script:      "/dist/home.js",
				CriticalCSS: ".hero{display:grid}",
				CSS:         "/dist/home.css",
				CSSFiles:    []string{"/dist/extra.css"},
			},
		},
	}

	html, err := svc.renderPageHTML(
		ServePageInput{Manifest: manifest, EntryName: "home"},
		map[string]any{"name": "World"},
		core.RenderedPage{
			Body: `<div class="hero">Hello</div>`,
			Head: `<title>Home</title>`,
		},
		"en",
		"",
	)
	if err != nil {
		t.Fatalf("renderPageHTML failed: %v", err)
	}

	if !strings.Contains(html, `data-bifrost-critical`) {
		t.Fatal("expected inline critical CSS tag")
	}
	if !strings.Contains(html, `href="/dist/home.css"`) || !strings.Contains(html, `href="/dist/extra.css"`) {
		t.Fatal("expected both stylesheet links")
	}
	if strings.Count(html, `media="print"`) != 2 {
		t.Fatal("expected deferred load for each non-critical stylesheet")
	}
}
