package bifrost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSitemapListsStaticRoutes(t *testing.T) {
	routes := []Route{
		Native("/{$}", staticNativeRender(NativePage{}), nil),
		Native("/pricing", staticNativeRender(NativePage{}), nil),
		Native("/blog/{slug}", staticNativeRender(NativePage{}), nil),
		Server("/docs/{$}", "page.tsx", nil),
		Server("/{path...}", "page.tsx", nil),
	}
	info, err := newSite("https://example.com", routes)
	if err != nil {
		t.Fatal(err)
	}
	sitemap := string(info.sitemap)
	for _, want := range []string{
		`<loc>https://example.com/</loc>`,
		`<loc>https://example.com/pricing</loc>`,
		`<loc>https://example.com/docs</loc>`,
	} {
		if !strings.Contains(sitemap, want) {
			t.Errorf("sitemap misses %q: %s", want, sitemap)
		}
	}
	if strings.Contains(sitemap, "slug") || strings.Contains(sitemap, "path...") {
		t.Errorf("sitemap lists a dynamic route: %s", sitemap)
	}
	if got, want := strings.Count(sitemap, "<url>"), 3; got != want {
		t.Errorf("%d urls, want %d", got, want)
	}
	if !strings.HasPrefix(sitemap, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Errorf("sitemap has no xml declaration: %s", sitemap)
	}
}

func TestSitemapKeepsBasePath(t *testing.T) {
	info, err := newSite("https://example.com/app/", []Route{Native("/{$}", staticNativeRender(NativePage{}), nil), Native("/pricing", staticNativeRender(NativePage{}), nil)})
	if err != nil {
		t.Fatal(err)
	}
	if sitemap := string(info.sitemap); !strings.Contains(sitemap, `https://example.com/app/pricing`) {
		t.Errorf("sitemap %s", sitemap)
	}
	if robots := string(info.robots); !strings.Contains(robots, `Sitemap: https://example.com/app/sitemap.xml`) {
		t.Errorf("robots %s", robots)
	}
}

func TestSiteURLValidation(t *testing.T) {
	cases := []string{"example.com", "/pricing", "ftp://example.com", "https://example.com?x=1", "https://user@example.com"}
	for _, value := range cases {
		if _, err := newSite(value, nil); err == nil {
			t.Errorf("site url %q was accepted", value)
		}
	}
	info, err := newSite("", nil)
	if err != nil || info != nil {
		t.Fatalf("empty site url: %v %v", info, err)
	}
}

func TestRegisterServesSitemap(t *testing.T) {
	app := newNativeApp(t, Config{
		SiteURL: "https://example.com",
		Routes: []Route{
			Native("/{$}", staticNativeRender(NativePage{Title: "Home"}), nil),
		},
	})
	mux := http.NewServeMux()
	if err := app.Register(mux); err != nil {
		t.Fatal(err)
	}

	sitemap := httptest.NewRecorder()
	mux.ServeHTTP(sitemap, httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil))
	if sitemap.Code != http.StatusOK {
		t.Fatalf("sitemap status %d", sitemap.Code)
	}
	if got := sitemap.Header().Get("Content-Type"); got != "application/xml; charset=utf-8" {
		t.Errorf("sitemap content type %q", got)
	}
	if !strings.Contains(sitemap.Body.String(), "<loc>https://example.com/</loc>") {
		t.Errorf("sitemap %s", sitemap.Body.String())
	}
	etag := sitemap.Header().Get("ETag")
	request := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	request.Header.Set("If-None-Match", etag)
	notModified := httptest.NewRecorder()
	mux.ServeHTTP(notModified, request)
	if notModified.Code != http.StatusNotModified {
		t.Errorf("second sitemap status %d", notModified.Code)
	}

	robots := httptest.NewRecorder()
	mux.ServeHTTP(robots, httptest.NewRequest(http.MethodGet, "/robots.txt", nil))
	if robots.Code != http.StatusOK {
		t.Fatalf("robots status %d", robots.Code)
	}
	if !strings.Contains(robots.Body.String(), "Sitemap: https://example.com/sitemap.xml") {
		t.Errorf("robots %s", robots.Body.String())
	}
}

func TestRegisterWithoutSiteURLServesNoSitemap(t *testing.T) {
	app := newNativeApp(t, Config{Routes: []Route{Native("/{$}", staticNativeRender(NativePage{}), nil)}})
	response := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status %d", response.Code)
	}
}

func TestNativeSocialAndCrawlerTags(t *testing.T) {
	render := staticNativeRender(NativePage{
		Title:     "Pricing",
		Image:     "https://example.com/card.png",
		Robots:    "noindex",
		Schema:    `{"@context":"https://schema.org","name":"</script><script>"}`,
		Lang:      "es",
		Dir:       "rtl",
		Canonical: "https://example.com/pricing",
		Body:      []byte("<h1>Pricing</h1>"),
	})
	app := newNativeApp(t, Config{Routes: []Route{Native("/pricing", render, nil)}})
	response := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/pricing", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status %d", response.Code)
	}
	body := response.Body.String()
	for _, want := range []string{
		`<html lang="es" dir="rtl">`,
		`<meta property="og:title" content="Pricing">`,
		`<meta property="og:url" content="https://example.com/pricing">`,
		`<meta property="og:image" content="https://example.com/card.png">`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`<meta name="robots" content="noindex">`,
		`<script type="application/ld+json">{"@context":"https://schema.org","name":"\u003c/script\u003e\u003cscript\u003e"}</script>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body misses %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "</script><script>") {
		t.Errorf("schema closes its script: %s", body)
	}
}

func TestNativeRejectsInvalidMetadata(t *testing.T) {
	cases := []NativePage{
		{Lang: "not a tag"},
		{Dir: "sideways"},
		{Schema: "not json"},
	}
	for _, page := range cases {
		render := func(context.Context, json.RawMessage) (NativePage, error) { return page, nil }
		app := newNativeApp(t, Config{Routes: []Route{Native("/{$}", render, nil)}})
		response := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/", nil))
		if response.Code != http.StatusInternalServerError {
			t.Errorf("page %+v answered %d", page, response.Code)
		}
	}
}

func TestSitemapSkipsNoIndexRoutes(t *testing.T) {
	info, err := newSite("https://example.com", []Route{
		Native("/{$}", staticNativeRender(NativePage{}), nil),
		Native("/settings", staticNativeRender(NativePage{}), nil).WithNoIndex(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if sitemap := string(info.sitemap); strings.Contains(sitemap, "/settings") {
		t.Errorf("sitemap lists a noindex route: %s", sitemap)
	}
}

func TestSitemapListsThePathsARouteAdds(t *testing.T) {
	info, err := newSite("https://example.com", []Route{
		Native("/{$}", staticNativeRender(NativePage{}), nil),
		Native("/blog/{slug}", staticNativeRender(NativePage{}), nil).
			WithSitemap("/blog/hello", "/blog/bye/", "/blog/hello"),
	})
	if err != nil {
		t.Fatal(err)
	}
	sitemap := string(info.sitemap)
	for _, want := range []string{`<loc>https://example.com/blog/bye</loc>`, `<loc>https://example.com/blog/hello</loc>`} {
		if !strings.Contains(sitemap, want) {
			t.Errorf("sitemap misses %q: %s", want, sitemap)
		}
	}
	if got, want := strings.Count(sitemap, "<url>"), 3; got != want {
		t.Errorf("%d urls, want %d", got, want)
	}
}

func TestSitemapRejectsAPathThatIsNotOne(t *testing.T) {
	for _, path := range []string{"blog/hello", "/blog/hello?x=1", "/blog/{slug}", "/blog/#top", "/blog/a b"} {
		_, err := newSite("https://example.com", []Route{
			Native("/blog/{slug}", staticNativeRender(NativePage{}), nil).WithSitemap(path),
		})
		if err == nil {
			t.Errorf("path %q passed", path)
		}
	}
}

func TestNativeKeywordsTypeAndAlternates(t *testing.T) {
	render := staticNativeRender(NativePage{
		Title:    "Pricing",
		Type:     "article",
		Keywords: "pricing, plans",
		Schema:   `[{"@type":"Organization","name":"Bifrost"},{"@type":"WebSite","name":"Bifrost"}]`,
		Alternates: []Alternate{
			{Lang: "es", URL: "https://example.com/es/pricing"},
			{Lang: "x-default", URL: "https://example.com/pricing"},
		},
		Body: []byte("<h1>Pricing</h1>"),
	})
	app := newNativeApp(t, Config{Routes: []Route{Native("/pricing", render, nil)}})
	body := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/pricing", nil)).Body.String()
	for _, want := range []string{
		`<meta property="og:type" content="article">`,
		`<meta name="keywords" content="pricing, plans">`,
		`<link rel="alternate" hreflang="es" href="https://example.com/es/pricing">`,
		`<link rel="alternate" hreflang="x-default" href="https://example.com/pricing">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body misses %q:\n%s", want, body)
		}
	}
	if got := strings.Count(body, `<script type="application/ld+json">`); got != 2 {
		t.Errorf("%d schema scripts, want 2:\n%s", got, body)
	}
	if !strings.Contains(body, `<meta property="og:type" content="article">`) {
		t.Errorf("og:type does not follow the page: %s", body)
	}
}

func TestNativeTypeDefaultsToWebsite(t *testing.T) {
	render := staticNativeRender(NativePage{Title: "Home", Body: []byte("<h1>Home</h1>")})
	app := newNativeApp(t, Config{Routes: []Route{Native("/{$}", render, nil)}})
	body := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/", nil)).Body.String()
	if !strings.Contains(body, `<meta property="og:type" content="website">`) {
		t.Errorf("body misses og:type:\n%s", body)
	}
}

func TestNativeRejectsInvalidAlternates(t *testing.T) {
	cases := [][]Alternate{
		{{Lang: "not a tag", URL: "https://example.com"}},
		{{Lang: "es"}},
	}
	for _, alternates := range cases {
		render := staticNativeRender(NativePage{Title: "Home", Alternates: alternates, Body: []byte("<h1>Home</h1>")})
		app := newNativeApp(t, Config{Routes: []Route{Native("/{$}", render, nil)}})
		if response := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/", nil)); response.Code != http.StatusInternalServerError {
			t.Errorf("alternates %+v answered %d", alternates, response.Code)
		}
	}
}

func TestSitemapEntriesCarryLastModAndAlternates(t *testing.T) {
	lastMod := time.Date(2025, 3, 2, 10, 0, 0, 0, time.UTC)
	route := Native("/blog", staticNativeRender(NativePage{}), nil).WithSitemapEntries(
		SitemapEntry{
			Path:    "/blog/seo-sin-navegador",
			LastMod: lastMod,
			Alternates: []Alternate{
				{Lang: "es", URL: "https://example.com/es/blog/seo-sin-navegador"},
				{Lang: "x-default", URL: "https://example.com/blog/seo-sin-navegador"},
			},
		},
		SitemapEntry{Path: "/blog/otro/"},
	)
	info, err := newSite("https://example.com", []Route{route})
	if err != nil {
		t.Fatal(err)
	}
	sitemap := string(info.sitemap)
	for _, want := range []string{
		`<loc>https://example.com/blog/seo-sin-navegador</loc><lastmod>2025-03-02T10:00:00Z</lastmod>`,
		`<xhtml:link rel="alternate" hreflang="es" href="https://example.com/es/blog/seo-sin-navegador"/>`,
		`<xhtml:link rel="alternate" hreflang="x-default" href="https://example.com/blog/seo-sin-navegador"/>`,
		`<loc>https://example.com/blog/otro</loc></url>`,
	} {
		if !strings.Contains(sitemap, want) {
			t.Errorf("sitemap misses %q: %s", want, sitemap)
		}
	}
	if !strings.Contains(sitemap, `xmlns:xhtml="http://www.w3.org/1999/xhtml"`) {
		t.Errorf("sitemap has no xhtml namespace: %s", sitemap)
	}
	if strings.Contains(sitemap, `<loc>https://example.com/blog/otro</loc><lastmod>`) {
		t.Errorf("an entry without a date wrote one: %s", sitemap)
	}
}

func TestSitemapSkipsTheNamespaceWithoutAlternates(t *testing.T) {
	info, err := newSite("https://example.com", []Route{Native("/pricing", staticNativeRender(NativePage{}), nil)})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(info.sitemap), "xhtml") {
		t.Errorf("sitemap declares a namespace nothing uses: %s", info.sitemap)
	}
}

func TestSitemapKeepsTheLastPageOfAPath(t *testing.T) {
	route := Native("/pricing", staticNativeRender(NativePage{}), nil).WithSitemapEntries(
		SitemapEntry{Path: "/pricing", LastMod: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
	)
	info, err := newSite("https://example.com", []Route{route})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Count(string(info.sitemap), "<url>"), 1; got != want {
		t.Fatalf("%d urls, want %d: %s", got, want, info.sitemap)
	}
	if !strings.Contains(string(info.sitemap), `<lastmod>2025-01-02T00:00:00Z</lastmod>`) {
		t.Errorf("the entry of the pattern lost its date: %s", info.sitemap)
	}
}

func TestSitemapRejectsAnAlternateThatIsNotAURL(t *testing.T) {
	route := Native("/blog", staticNativeRender(NativePage{}), nil).WithSitemapEntries(
		SitemapEntry{Path: "/blog/a", Alternates: []Alternate{{Lang: "es", URL: "/es/blog/a"}}},
	)
	if _, err := newSite("https://example.com", []Route{route}); err == nil {
		t.Fatal("a relative alternate was taken")
	}
	route = Native("/blog", staticNativeRender(NativePage{}), nil).WithSitemapEntries(
		SitemapEntry{Path: "/blog/a", Alternates: []Alternate{{Lang: "not a tag", URL: "https://example.com/a"}}},
	)
	if _, err := newSite("https://example.com", []Route{route}); err == nil {
		t.Fatal("a bad language was taken")
	}
}
