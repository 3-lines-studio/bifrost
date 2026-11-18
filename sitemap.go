package bifrost

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// site is what a site tells crawlers when Config.SiteURL names it: the pages
// anyone can reach, and where the sitemap of those pages is.
type site struct {
	sitemap []byte
	robots  []byte
}

func newSite(siteURL string, routes []Route) (*site, error) {
	if siteURL == "" {
		return nil, nil
	}
	base, err := url.Parse(strings.TrimSuffix(siteURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("bifrost: Config.SiteURL: %w", err)
	}
	if base.Scheme != "http" && base.Scheme != "https" || base.Host == "" {
		return nil, errors.New("bifrost: Config.SiteURL must be an absolute http or https URL")
	}
	if base.RawQuery != "" || base.Fragment != "" || base.User != nil {
		return nil, errors.New("bifrost: Config.SiteURL must not have a query, fragment, or user")
	}
	pages := make([]sitemapPage, 0, len(routes))
	for _, route := range routes {
		if route.pattern == "" || route.noIndex {
			continue
		}
		path := strings.TrimSuffix(route.pattern, "/{$}")
		if path == "" {
			path = "/"
		}
		if !strings.ContainsAny(path, "{") {
			pages = appendSitemapPage(pages, sitemapPage{Path: path})
		}
		for _, extra := range route.sitemap {
			resolved, err := sitemapPageOf(extra)
			if err != nil {
				return nil, fmt.Errorf("bifrost: route %q: %w", route.pattern, err)
			}
			pages = appendSitemapPage(pages, resolved)
		}
	}
	slices.SortFunc(pages, func(a, b sitemapPage) int { return strings.Compare(a.Path, b.Path) })

	var sitemap strings.Builder
	sitemap.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"`)
	if slices.ContainsFunc(pages, func(page sitemapPage) bool { return len(page.Alternates) > 0 }) {
		sitemap.WriteString(` xmlns:xhtml="http://www.w3.org/1999/xhtml"`)
	}
	sitemap.WriteString(`>`)
	for _, page := range pages {
		sitemap.WriteString(`<url><loc>`)
		sitemap.WriteString(html.EscapeString(siteAddress(base, page.Path)))
		sitemap.WriteString(`</loc>`)
		if !page.LastMod.IsZero() {
			sitemap.WriteString(`<lastmod>`)
			sitemap.WriteString(page.LastMod.Format(time.RFC3339))
			sitemap.WriteString(`</lastmod>`)
		}
		for _, alternate := range page.Alternates {
			sitemap.WriteString(`<xhtml:link rel="alternate" hreflang="`)
			sitemap.WriteString(html.EscapeString(alternate.Lang))
			sitemap.WriteString(`" href="`)
			sitemap.WriteString(html.EscapeString(alternate.URL))
			sitemap.WriteString(`"/>`)
		}
		sitemap.WriteString(`</url>`)
	}
	sitemap.WriteString(`</urlset>`)

	var robots strings.Builder
	robots.WriteString("User-agent: *\nAllow: /\n")
	robots.WriteString("Sitemap: " + siteAddress(base, "/sitemap.xml") + "\n")
	return &site{sitemap: []byte(sitemap.String()), robots: []byte(robots.String())}, nil
}

// siteAddress joins a path to the site's origin, keeping a base path.
func siteAddress(base *url.URL, path string) string {
	return strings.TrimSuffix(base.String(), "/") + path
}

// sitemapPath checks a path a route adds to the sitemap, and answers it with a
// leading slash and no trailing one.
func sitemapPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("sitemap path %q must start with a slash", path)
	}
	if strings.ContainsAny(path, "?#{ }") {
		return "", fmt.Errorf("sitemap path %q must be a path without a parameter, query, or fragment", path)
	}
	if len(path) > 1 {
		path = strings.TrimSuffix(path, "/")
	}
	return path, nil
}

// sitemapPage is one page of the sitemap as it is written.
type sitemapPage struct {
	Path       string
	LastMod    time.Time
	Alternates []Alternate
}

// sitemapPageOf checks an entry a route adds to the sitemap and answers it as a
// page of it.
func sitemapPageOf(entry SitemapEntry) (sitemapPage, error) {
	path, err := sitemapPath(entry.Path)
	if err != nil {
		return sitemapPage{}, err
	}
	if err := sitemapAlternates(entry.Alternates); err != nil {
		return sitemapPage{}, err
	}
	return sitemapPage{Path: path, LastMod: entry.LastMod, Alternates: entry.Alternates}, nil
}

// sitemapAlternates checks the languages a page comes in: a language tag, or
// x-default, and the absolute URL of the page in it.
func sitemapAlternates(alternates []Alternate) error {
	for _, alternate := range alternates {
		if alternate.Lang != "x-default" && !validLanguageTag(alternate.Lang) {
			return fmt.Errorf("sitemap alternate has an invalid language %q", alternate.Lang)
		}
		address, err := url.Parse(alternate.URL)
		if err != nil || address.Host == "" || (address.Scheme != "http" && address.Scheme != "https") {
			return fmt.Errorf("sitemap alternate %q must be an absolute http or https URL", alternate.Lang)
		}
	}
	return nil
}

// appendSitemapPage adds page to pages, or replaces the one of its path.
func appendSitemapPage(pages []sitemapPage, page sitemapPage) []sitemapPage {
	if at := slices.IndexFunc(pages, func(known sitemapPage) bool { return known.Path == page.Path }); at >= 0 {
		pages[at] = page
		return pages
	}
	return append(pages, page)
}

func (a *App) serveSitemap(w http.ResponseWriter, request *http.Request) {
	serveSiteFile(w, request, "application/xml; charset=utf-8", a.site.sitemap)
}

func (a *App) serveRobots(w http.ResponseWriter, request *http.Request) {
	serveSiteFile(w, request, "text/plain; charset=utf-8", a.site.robots)
}

func serveSiteFile(w http.ResponseWriter, request *http.Request, contentType string, body []byte) {
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("ETag", etag)
	if request.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(body)
}
