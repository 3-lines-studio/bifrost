package bifrost

import (
	"context"
	"encoding/json"
	"net/http"
)

type Loader func(*http.Request) (any, error)

type RawProps json.RawMessage

type Document struct {
	Lang  string
	Class string
	Dir   string
}

type PageData struct {
	Props          any
	Document       Document
	Status         int
	ErrorFallbacks int
}

type StaticPage struct {
	Path     string
	Props    any
	Document Document
}

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

type Route struct {
	pattern      string
	view         string
	kind         routeKind
	load         Loader
	generate     Generator
	navigation   bool
	cacheControl string
}

func Server(pattern, view string, load Loader) Route {
	return Route{
		pattern: pattern,
		view:    view,
		kind:    routeServer,
		load:    load,
	}
}

func Static(pattern, view string, generate Generator) Route {
	return Route{
		pattern:  pattern,
		view:     view,
		kind:     routeStatic,
		generate: generate,
	}
}

func Client(pattern, view string) Route {
	return Route{
		pattern: pattern,
		view:    view,
		kind:    routeClient,
	}
}

func (r Route) WithNavigation() Route {
	r.navigation = true
	return r
}

func (r Route) WithCache(cacheControl string) Route {
	r.cacheControl = cacheControl
	return r
}
