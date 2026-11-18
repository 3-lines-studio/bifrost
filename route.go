package bifrost

import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
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
type PageData struct {
	Props          any
	Document       Document
	Status         int
	ErrorFallbacks int
}

// StaticPage describes one document emitted by a static route.
type StaticPage struct {
	Path     string
	Props    any
	Document Document
}

// Generator returns all documents emitted by a static route.
type Generator func(context.Context) ([]StaticPage, error)

type routeKind uint8

const (
	routeInvalid routeKind = iota
	routeServer
	routeStatic
	routeClient
	routeHTML
)

// Asset-only kinds reuse routeKind so a path is validated the same way whether
// it is a template, a stylesheet, or a script.
const (
	routeCSS    routeKind = 200
	routeScript routeKind = 201
)

func (k routeKind) String() string {
	switch k {
	case routeServer:
		return "server"
	case routeStatic:
		return "static"
	case routeClient:
		return "client"
	case routeHTML:
		return "html"
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
	navigation   bool
	cacheControl string
	html         *HTMLPage
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

// HTML declares a page the process serves from Go templates: no renderer, no
// hydration, no JavaScript runtime unless the page brings a script of its own.
// The template is the document the browser receives.
func HTML(pattern string, page HTMLPage) Route {
	return Route{
		pattern: pattern,
		view:    page.Page,
		kind:    routeHTML,
		load:    page.Load,
		html:    &page,
	}
}

// HTMLPage describes an App Router HTML page: one page template, the layouts
// that wrap it, and the loader that feeds both.
//
// Page and Layouts are paths inside Config.Templates, in the same forward-slash
// form as every other view. Layouts are applied outermost first, so the last
// entry is the layout closest to the page.
type HTMLPage struct {
	Page    string
	Layouts []string

	// Load returns the data the page template reads. It may return a PageData to
	// set the document language, attributes, and status.
	Load Loader

	// LayoutData is the value every layout template reads as Frame.Data. Page
	// data feeds the page; the chrome gets its own, so it does not ride along in
	// every page's data.
	LayoutData Loader

	// Metadata builds the document head. An empty func leaves the head to the
	// templates' static content.
	Metadata MetadataFunc

	// LayoutMetadata builds the part of the head that belongs to the chrome: the
	// theme script, the preloads, the tags every page shares. A field the page
	// also sets wins, and both Head fragments are kept, the layout first.
	LayoutMetadata MetadataFunc

	// Params names the dynamic segments of the pattern. Each one is read from the
	// request and exposed to every template as Frame.Params.
	Params []string

	// Styles and Scripts are the files this page needs, relative to the source
	// root. They are build inputs: the build bundles them and the document links
	// the result.
	Styles  []string
	Scripts []string

	// LayoutStyles and LayoutScripts belong to the chrome. The build emits them
	// as their own assets, shared by every page under the same layout, so a page
	// ships only what is its own and the browser caches the rest once.
	LayoutStyles  []string
	LayoutScripts []string
}

// MetadataFunc builds the head of one HTML document.
type MetadataFunc func(*http.Request) (Metadata, error)

// Metadata is the head of an HTML page. The zero value emits no tags, which is
// the right head for a page that only needs its document language.
type Metadata struct {
	Title       string
	Description string
	Keywords    string
	Robots      string
	Canonical   string
	Image       string
	ImageAlt    string
	Type        string
	Locale      string

	// Head is trusted markup appended after the tags above. It is the only field
	// that is not escaped, so JSON-LD belongs here.
	Head template.HTML
}

// Frame is what every layout template reads. The page template reads the
// loader's value directly, so a page never pays for the wrapper.
type Frame struct {
	Data     any
	Children template.HTML
	Path     string
	Query    url.Values
	Params   map[string]string
}

func (r Route) WithNavigation() Route {
	r.navigation = true
	return r
}

// WithCache sets the Cache-Control header of the server-rendered document.
// Routes default to private, no-cache, no-store, max-age=0, must-revalidate,
// which keeps browsers and shared caches from serving a page whose props came
// from a request. The client-side navigation
// response is never cacheable, and a static route caches the HTML the build
// prerendered.
func (r Route) WithCache(cacheControl string) Route {
	r.cacheControl = cacheControl
	return r
}
