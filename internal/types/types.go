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

type StaticPathData struct {
	Path  string
	Props map[string]any
}

type StaticDataLoader func(context.Context) ([]StaticPathData, error)

type PageConfig struct {
	ComponentPath    string
	Mode             PageMode
	PropsLoader      PropsLoader
	StaticDataLoader StaticDataLoader
}

type PageOption func(*PageConfig)

func WithLoader(loader PropsLoader) PageOption {
	return func(c *PageConfig) {
		c.PropsLoader = loader
	}
}

func WithClient() PageOption {
	return func(c *PageConfig) {
		c.Mode = ModeClientOnly
	}
}

func WithStatic() PageOption {
	return func(c *PageConfig) {
		c.Mode = ModeStaticPrerender
	}
}

func WithStaticData(loader StaticDataLoader) PageOption {
	return func(c *PageConfig) {
		c.Mode = ModeStaticPrerender
		c.StaticDataLoader = loader
	}
}

type RenderedPage struct {
	Body string
	Head string
}
