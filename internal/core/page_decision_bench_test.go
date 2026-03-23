package core

import "testing"

func BenchmarkDecidePageAction_ProdSSR(b *testing.B) {
	req := PageRequest{IsDev: false, Mode: ModeSSR, RequestPath: "/about"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		DecidePageAction(req, nil)
	}
}

func BenchmarkDecidePageAction_ProdStaticPrerender_Hit(b *testing.B) {
	entry := &ManifestEntry{
		StaticRoutes: map[string]string{
			"/blog/hello": "/pages/routes/blog/hello/index.html",
		},
	}
	req := PageRequest{IsDev: false, Mode: ModeStaticPrerender, RequestPath: "/blog/hello", HasManifest: true}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		DecidePageAction(req, entry)
	}
}

func BenchmarkDecidePageAction_DevSSR(b *testing.B) {
	req := PageRequest{IsDev: true, Mode: ModeSSR, HasRenderer: true}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		DecidePageAction(req, nil)
	}
}

func BenchmarkNormalizePath(b *testing.B) {
	paths := []string{"/about", "about/", "/blog/hello", "/"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		NormalizePath(paths[i%len(paths)])
	}
}

func BenchmarkGetAssets_Hit(b *testing.B) {
	man := &Manifest{
		Entries: map[string]ManifestEntry{
			"pages-home-entry": {
				Script: "/dist/pages-home-entry-abc.js",
				CSS:    "/dist/pages-home-entry-abc.css",
				Chunks: []string{"/dist/chunk-1.js"},
			},
		},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		GetAssets(man, "pages-home-entry")
	}
}

func BenchmarkGetAssets_Fallback(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		GetAssets(nil, "pages-home-entry")
	}
}

func BenchmarkResolvePageArtifacts_Hit(b *testing.B) {
	man := &Manifest{
		Entries: map[string]ManifestEntry{
			"pages-home-entry": {
				Script: "/dist/pages-home-entry-abc.js",
				CSS:    "/dist/pages-home-entry-abc.css",
				Chunks: []string{"/dist/chunk-1.js"},
			},
		},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ResolvePageArtifacts(man, "pages-home-entry")
	}
}

func BenchmarkResolvePageArtifacts_Fallback(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ResolvePageArtifacts(nil, "pages-home-entry")
	}
}

func BenchmarkGetContentType(b *testing.B) {
	paths := []string{"style.css", "app.js", "image.PNG", "font.woff2", "data.bin"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		GetContentType(paths[i%len(paths)])
	}
}

func BenchmarkEntryNameForPath(b *testing.B) {
	paths := []string{"./pages/home.tsx", "pages/about.tsx", "./pages/nested/page.tsx"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		EntryNameForPath(paths[i%len(paths)])
	}
}
