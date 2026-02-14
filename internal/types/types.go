package types

import (
	"context"
	"net/http"
)

type PropsLoader func(*http.Request) (map[string]any, error)

type RedirectError interface {
	RedirectURL() string
	RedirectStatusCode() int
}

type PageMode int

const (
	ModeSSR PageMode = iota
	ModeClientOnly
	ModeStaticPrerender
)

// StaticPathData represents a single path and its props for static prerendering
type StaticPathData struct {
	Path  string
	Props map[string]any
}

// StaticDataLoader is called at build time to generate dynamic static paths
// It returns a list of paths and their corresponding props
// Example: returning [{Path: "/blog/hello", Props: {slug: "hello"}}]
// will generate a prerendered page at /blog/hello with those props
type StaticDataLoader func(context.Context) ([]StaticPathData, error)

type PageConfig struct {
	ComponentPath    string
	Mode             PageMode
	PropsLoader      PropsLoader
	StaticDataLoader StaticDataLoader
}

type PageOption func(*PageConfig)

func WithPropsLoader(loader PropsLoader) PageOption {
	return func(c *PageConfig) {
		c.PropsLoader = loader
	}
}

func WithClientOnly() PageOption {
	return func(c *PageConfig) {
		c.Mode = ModeClientOnly
	}
}

func WithStaticPrerender() PageOption {
	return func(c *PageConfig) {
		c.Mode = ModeStaticPrerender
	}
}

func WithStaticDataLoader(loader StaticDataLoader) PageOption {
	return func(c *PageConfig) {
		c.StaticDataLoader = loader
	}
}

type RenderedPage struct {
	Body string
	Head string
}
