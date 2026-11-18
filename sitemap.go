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
	paths := make([]string, 0, len(routes))
	for _, route := range routes {
		if route.pattern == "" || route.noIndex {
			continue
		}
		path := strings.TrimSuffix(route.pattern, "/{$}")
		if path == "" {
			path = "/"
		}
		if !strings.ContainsAny(path, "{") {
			paths = appendPath(paths, path)
		}
		for _, extra := range route.sitemap {
			resolved, err := sitemapPath(extra)
			if err != nil {
				return nil, fmt.Errorf("bifrost: route %q: %w", route.pattern, err)
			}
			paths = appendPath(paths, resolved)
		}
	}
	slices.Sort(paths)

	var sitemap strings.Builder
	sitemap.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, path := range paths {
		sitemap.WriteString(`<url><loc>`)
		sitemap.WriteString(html.EscapeString(siteAddress(base, path)))
		sitemap.WriteString(`</loc></url>`)
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

func appendPath(paths []string, path string) []string {
	if slices.Contains(paths, path) {
		return paths
	}
	return append(paths, path)
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
