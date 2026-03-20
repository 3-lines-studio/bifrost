package core

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
	HTMLLang         string
	HTMLClass        string
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

func WithHTMLLang(lang string) PageOption {
	return func(c *PageConfig) {
		c.HTMLLang = lang
	}
}

func WithHTMLClass(class string) PageOption {
	return func(c *PageConfig) {
		c.HTMLClass = class
	}
}

type RenderedPage struct {
	Body string
	Head string
}

type Mode int

const (
	ModeDev Mode = iota
	ModeProd
	ModeExport
)

type Renderer interface {
	Render(componentPath string, props map[string]any) (RenderedPage, error)
	Build(entrypoints []string, outdir string) error
}

type Config struct {
	Framework       Framework
	DefaultHTMLLang string
}

type ConfigOption func(*Config)

func WithFramework(fw Framework) ConfigOption {
	return func(c *Config) {
		c.Framework = fw
	}
}

func WithDefaultHTMLLang(lang string) ConfigOption {
	return func(c *Config) {
		c.DefaultHTMLLang = lang
	}
}
