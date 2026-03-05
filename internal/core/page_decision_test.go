package core

import (
	"testing"
)

func TestDecidePageAction_DevSSR(t *testing.T) {
	req := PageRequest{
		IsDev:       true,
		Mode:        ModeSSR,
		HasRenderer: true,
	}
	decision := DecidePageAction(req, nil)
	if decision.Action != ActionNeedsSetup {
		t.Errorf("expected ActionNeedsSetup, got %d", decision.Action)
	}
}

func TestDecidePageAction_DevClientOnly_NoRenderer(t *testing.T) {
	req := PageRequest{
		IsDev:       true,
		Mode:        ModeClientOnly,
		HasRenderer: false,
	}
	decision := DecidePageAction(req, nil)
	if decision.Action != ActionRenderClientOnlyShell {
		t.Errorf("expected ActionRenderClientOnlyShell, got %d", decision.Action)
	}
}

func TestDecidePageAction_DevStaticPrerender_NoRenderer(t *testing.T) {
	req := PageRequest{
		IsDev:       true,
		Mode:        ModeStaticPrerender,
		HasRenderer: false,
	}
	decision := DecidePageAction(req, nil)
	if decision.Action != ActionRenderStaticPrerender {
		t.Errorf("expected ActionRenderStaticPrerender, got %d", decision.Action)
	}
}

func TestDecidePageAction_ProdSSR(t *testing.T) {
	req := PageRequest{
		IsDev: false,
		Mode:  ModeSSR,
	}
	decision := DecidePageAction(req, nil)
	if decision.Action != ActionRenderSSR {
		t.Errorf("expected ActionRenderSSR, got %d", decision.Action)
	}
}

func TestDecidePageAction_ProdClientOnly_WithHTML(t *testing.T) {
	entry := &ManifestEntry{HTML: "/pages/about.html"}
	req := PageRequest{
		IsDev:       false,
		Mode:        ModeClientOnly,
		HasManifest: true,
	}
	decision := DecidePageAction(req, entry)
	if decision.Action != ActionServeStaticFile {
		t.Errorf("expected ActionServeStaticFile, got %d", decision.Action)
	}
	if decision.StaticPath != "/pages/about.html" {
		t.Errorf("expected /pages/about.html, got %s", decision.StaticPath)
	}
}

func TestDecidePageAction_ProdClientOnly_NoHTML(t *testing.T) {
	req := PageRequest{
		IsDev:       false,
		Mode:        ModeClientOnly,
		HasManifest: true,
	}
	decision := DecidePageAction(req, nil)
	if decision.Action != ActionNotFound {
		t.Errorf("expected ActionNotFound, got %d", decision.Action)
	}
}

func TestDecidePageAction_ProdStaticPrerender_MatchedRoute(t *testing.T) {
	entry := &ManifestEntry{
		StaticRoutes: map[string]string{
			"/blog/hello": "/pages/routes/blog/hello/index.html",
		},
	}
	req := PageRequest{
		IsDev:       false,
		Mode:        ModeStaticPrerender,
		RequestPath: "/blog/hello",
		HasManifest: true,
	}
	decision := DecidePageAction(req, entry)
	if decision.Action != ActionServeRouteFile {
		t.Errorf("expected ActionServeRouteFile, got %d", decision.Action)
	}
	if decision.HTMLPath != "/pages/routes/blog/hello/index.html" {
		t.Errorf("unexpected html path: %s", decision.HTMLPath)
	}
}

func TestDecidePageAction_ProdStaticPrerender_UnmatchedRoute(t *testing.T) {
	entry := &ManifestEntry{
		StaticRoutes: map[string]string{
			"/blog/hello": "/pages/routes/blog/hello/index.html",
		},
	}
	req := PageRequest{
		IsDev:       false,
		Mode:        ModeStaticPrerender,
		RequestPath: "/blog/missing",
		HasManifest: true,
	}
	decision := DecidePageAction(req, entry)
	if decision.Action != ActionNotFound {
		t.Errorf("expected ActionNotFound for unmatched route, got %d", decision.Action)
	}
}

func TestDecidePageAction_ProdStaticPrerender_NoManifest(t *testing.T) {
	req := PageRequest{
		IsDev:       false,
		Mode:        ModeStaticPrerender,
		HasManifest: false,
	}
	decision := DecidePageAction(req, nil)
	if decision.Action != ActionNotFound {
		t.Errorf("expected ActionNotFound when no manifest in prod, got %d", decision.Action)
	}
}

func TestDecidePageAction_ProdStaticPrerender_WithStaticPath(t *testing.T) {
	req := PageRequest{
		IsDev:       false,
		Mode:        ModeStaticPrerender,
		HasManifest: false,
		StaticPath:  "/tmp/ssr/pages-home-entry-ssr.js",
	}
	decision := DecidePageAction(req, nil)
	if decision.Action != ActionServeStaticFile {
		t.Errorf("expected ActionServeStaticFile with static path, got %d", decision.Action)
	}
}

func TestDecidePageAction_ProdStaticPrerender_TrailingSlash(t *testing.T) {
	entry := &ManifestEntry{
		StaticRoutes: map[string]string{
			"/about": "/pages/routes/about/index.html",
		},
	}
	req := PageRequest{
		IsDev:       false,
		Mode:        ModeStaticPrerender,
		RequestPath: "/about/",
		HasManifest: true,
	}
	decision := DecidePageAction(req, entry)
	if decision.Action != ActionServeRouteFile {
		t.Errorf("expected ActionServeRouteFile for /about/, got %d", decision.Action)
	}
}

func TestResolveRenderPath(t *testing.T) {
	tests := []struct {
		name          string
		isDev         bool
		ssrPath       string
		componentPath string
		want          string
	}{
		{"dev returns component", true, "/ssr/page.js", "./pages/home.tsx", "./pages/home.tsx"},
		{"prod with ssr path", false, "/ssr/page.js", "./pages/home.tsx", "/ssr/page.js"},
		{"prod without ssr path", false, "", "./pages/home.tsx", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveRenderPath(tt.isDev, tt.ssrPath, tt.componentPath)
			if got != tt.want {
				t.Errorf("ResolveRenderPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMatchStaticRoute(t *testing.T) {
	manifest := &Manifest{
		Entries: map[string]ManifestEntry{
			"pages-blog-entry": {
				StaticRoutes: map[string]string{
					"/blog/hello":          "/pages/routes/blog/hello/index.html",
					"/blog/getting-started": "/pages/routes/blog/getting-started/index.html",
				},
			},
		},
	}

	t.Run("found", func(t *testing.T) {
		path, found := MatchStaticRoute(manifest, "pages-blog-entry", "/blog/hello")
		if !found {
			t.Error("expected to find route")
		}
		if path != "/pages/routes/blog/hello/index.html" {
			t.Errorf("unexpected path: %s", path)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, found := MatchStaticRoute(manifest, "pages-blog-entry", "/blog/missing")
		if found {
			t.Error("expected route not to be found")
		}
	})

	t.Run("nil manifest", func(t *testing.T) {
		_, found := MatchStaticRoute(nil, "pages-blog-entry", "/blog/hello")
		if found {
			t.Error("expected false for nil manifest")
		}
	})

	t.Run("missing entry", func(t *testing.T) {
		_, found := MatchStaticRoute(manifest, "nonexistent", "/blog/hello")
		if found {
			t.Error("expected false for missing entry")
		}
	})
}
