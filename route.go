package bifrost

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"time"
)

// Loader returns JSON-encodable props for one server-rendered request.
type Loader func(*http.Request) (any, error)

// RawProps lets an advanced loader provide pre-encoded JSON. Bifrost validates,
// compacts, and safely escapes it before rendering.
type RawProps json.RawMessage

// Document describes request-scoped attributes on the root HTML element.
// Empty Lang defaults to "en". Dir may be empty, "ltr", "rtl", or "auto".
type Document struct {
	Lang  string
	Class string
	Dir   string
}

// PageData lets a Server loader return props and root document attributes.
// A loader may still return plain props when it needs no document attributes.
//
// Headers go with the page it answers, before the ones of the answer itself:
// the cookies of a session, a Vary, a header of its own. Bifrost keeps the ones
// that describe the answer (Content-Type, Content-Length, Cache-Control,
// Transfer-Encoding, Connection and the ones of a proxy) to itself: a route
// caches with Cache(), not with a header.
type PageData struct {
	Props          any
	Document       Document
	Status         int
	ErrorFallbacks int
	Headers        http.Header
}

// StaticPage describes one document emitted by a static route.
type StaticPage struct {
	Path     string
	Props    any
	Document Document
}

// Generator returns all documents emitted by a static route.
type Generator func(context.Context) ([]StaticPage, error)

// NativePage is the document a Native route returns for one request. The body
// is HTML that Bifrost places inside the document it composes. The fields past
// Canonical are optional: Image becomes the social card, Robots overrides the
// crawler directives ("noindex" for a screen nobody searches), Schema is a
// JSON-LD value, Keywords is what a search engine reads of the subject, Type
// is the social card's og:type ("website" when empty), Alternates are the same
// page in other languages, and Lang and Dir override the document's attributes.
type NativePage struct {
	Title       string
	Description string
	Canonical   string
	Image       string
	Robots      string
	Schema      string
	Keywords    string
	Type        string
	Lang        string
	Dir         string
	Alternates  []Alternate
	Body        []byte
}

// Alternate is another language of a Native page, as a crawler reads it from a
// link element. Lang is a language tag as "es" or "es-AR", or "x-default" for
// the page served when no language matches.
type Alternate struct {
	Lang string
	URL  string
}

// NativeRender builds a Native page from the JSON props of its loader.
type NativeRender func(context.Context, json.RawMessage) (NativePage, error)

// NativeError builds the page a Native route answers when its loader fails.
// The status is the one the error wraps, or 500.
type NativeError func(context.Context, error) (NativePage, error)

type routeKind uint8

const (
	routeInvalid routeKind = iota
	routeServer
	routeStatic
	routeClient
	routeNative
)

func (k routeKind) String() string {
	switch k {
	case routeServer:
		return "server"
	case routeStatic:
		return "static"
	case routeClient:
		return "client"
	case routeNative:
		return "native"
	default:
		return "invalid"
	}
}

// Route is an immutable page declaration. Use Server, Static, or Client to
// construct one.
type Route struct {
	pattern      string
	view         string
	kind         routeKind
	load         Loader
	generate     Generator
	render       NativeRender
	navigation   bool
	cacheControl string
	noIndex      bool
	errorRender  NativeError
	sitemap      []SitemapEntry
}

// Server declares a page rendered on each request. A nil loader supplies empty
// props.
func Server(pattern, view string, load Loader) Route {
	return Route{
		pattern: pattern,
		view:    view,
		kind:    routeServer,
		load:    load,
	}
}

// Static declares a page rendered during the production build. A nil generator
// is valid only for an exact route and emits that route's own path.
func Static(pattern, view string, generate Generator) Route {
	return Route{
		pattern:  pattern,
		view:     view,
		kind:     routeStatic,
		generate: generate,
	}
}

// Client declares a page mounted only in the browser.
func Client(pattern, view string) Route {
	return Route{
		pattern: pattern,
		view:    view,
		kind:    routeClient,
	}
}

// Native declares a page rendered in-process by render. It needs no frontend
// build, no embedded assets, and no renderer process.
func Native(pattern string, render NativeRender, load Loader) Route {
	return Route{
		pattern: pattern,
		kind:    routeNative,
		render:  render,
		load:    load,
	}
}

func (r Route) WithNavigation() Route {
	r.navigation = true
	return r
}

// WithCache sets the Cache-Control header of the rendered document.
// Routes default to private, no-cache, no-store, max-age=0, must-revalidate,
// which keeps browsers and shared caches from serving a page whose props came
// from a request. The client-side navigation
// response is never cacheable, and a static route caches the HTML the build
// prerendered.
func (r Route) WithCache(cacheControl string) Route {
	r.cacheControl = cacheControl
	return r
}

// WithError sets the page a Native route answers when its loader fails: the
// status the error wraps, or 500, with the error handed to the render. A
// redirect still redirects, and a response hook still handles the error first.
func (r Route) WithError(render NativeError) Route {
	r.errorRender = render
	return r
}

// WithNoIndex keeps a route out of the sitemap: the pages it serves are
// screens of the app, not content a search engine indexes.
func (r Route) WithNoIndex() Route {
	r.noIndex = true
	return r
}

// SitemapEntry is one page a route adds to the sitemap: the path, when it last
// changed, and the same page in other languages. LastMod and Alternates are
// optional, and a zero LastMod writes no <lastmod>.
type SitemapEntry struct {
	Path       string
	LastMod    time.Time
	Alternates []Alternate
}

// WithSitemap adds the paths a route serves that its pattern cannot name, as
// the pages of a pattern with a parameter, such as every post of a blog. Bifrost
// lists them in the sitemap beside the routes whose pattern is a path.
func (r Route) WithSitemap(paths ...string) Route {
	entries := make([]SitemapEntry, 0, len(paths))
	for _, path := range paths {
		entries = append(entries, SitemapEntry{Path: path})
	}
	return r.WithSitemapEntries(entries...)
}

// WithSitemapEntries adds the pages a route serves, with when each last changed
// and the languages it comes in. A page of the same path written twice is the
// last one, and a page a pattern already names takes what the entry adds.
func (r Route) WithSitemapEntries(entries ...SitemapEntry) Route {
	r.sitemap = append(slices.Clone(r.sitemap), entries...)
	return r
}
