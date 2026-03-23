package usecase

import (
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func BenchmarkPageServiceRenderPageHTML_PrebuiltShell(b *testing.B) {
	b.ReportAllocs()

	manifest := &core.Manifest{
		Entries: map[string]core.ManifestEntry{
			"home": {
				Script:      "/dist/home.js",
				CriticalCSS: ".hero{display:grid}",
				CSS:         "/dist/home.css",
				CSSFiles:    []string{"/dist/extra.css"},
				Chunks:      []string{"/dist/chunk-a.js", "/dist/chunk-b.js"},
			},
		},
	}
	artifacts := core.ResolvePageArtifacts(manifest, "home")
	shell, err := core.NewHTMLDocumentShell(
		artifacts.Script,
		artifacts.CriticalCSS,
		core.StylesheetHrefsFor(artifacts),
		artifacts.Chunks,
	)
	if err != nil {
		b.Fatal(err)
	}

	svc := &PageService{}
	input := ServePageInput{
		Manifest:  manifest,
		EntryName: "home",
		Shell:     &shell,
	}
	page := core.RenderedPage{
		Body: `<div class="hero">Hello</div>`,
		Head: `<title>Home</title><meta name="description" content="bench" />`,
	}
	props := map[string]any{"name": "World", "count": 42}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.renderPageHTML(input, props, page, "en", "dark")
	}
}
