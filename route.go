package bifrost

import (
	"context"
	"encoding/json"
	"net/http"
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
	Props    any
	Document Document
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
)

func (k routeKind) String() string {
	switch k {
	case routeServer:
		return "server"
	case routeStatic:
		return "static"
	case routeClient:
		return "client"
	default:
		return "invalid"
	}
}

// Route is an immutable page declaration. Use Server, Static, or Client to
// construct one.
type Route struct {
	pattern  string
	view     string
	kind     routeKind
	load     Loader
	generate Generator
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
