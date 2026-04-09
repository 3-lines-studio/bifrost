package core

import (
	"context"
	"net/http"
)

type PropsLoader func(*http.Request) (map[string]any, error)

type DeferredPropsLoader func(*http.Request) (map[string]any, error)

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

func (m PageMode) IsStatic() bool {
	return m == ModeClientOnly || m == ModeStaticPrerender
}

func (m PageMode) NeedsSSRBundle() bool {
	return !m.IsStatic()
}

func (m PageMode) BuildLabel() string {
	switch m {
	case ModeClientOnly:
		return "client"
	case ModeStaticPrerender:
		return "static"
	default:
		return "ssr"
	}
}

func (m PageMode) DevAction(hasRenderer bool) PageDecision {
	if !m.IsStatic() || hasRenderer {
		return PageDecision{Action: ActionNeedsSetup, NeedsSetup: true}
	}
	return PageDecision{Action: m.RenderAction()}
}

func (m PageMode) RenderAction() PageAction {
	switch m {
	case ModeClientOnly:
		return ActionRenderClientOnlyShell
	case ModeStaticPrerender:
		return ActionRenderStaticPrerender
	default:
		return ActionRenderSSR
	}
}

type StaticPathData struct {
	Path  string
	Props map[string]any
}

type StaticDataLoader func(context.Context) ([]StaticPathData, error)

type PageConfig struct {
	ComponentPath       string
	Mode                PageMode
	PropsLoader         PropsLoader
	DeferredPropsLoader DeferredPropsLoader
	StaticDataLoader    StaticDataLoader
	HTMLLang            string
	HTMLClass           string
}

type PageOption func(*PageConfig)

func WithLoader(loader PropsLoader) PageOption {
	return func(c *PageConfig) {
		c.PropsLoader = loader
	}
}

func WithDeferredLoader(loader DeferredPropsLoader) PageOption {
	return func(c *PageConfig) {
		c.DeferredPropsLoader = loader
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

func MergeProps(sync map[string]any, deferred map[string]any) map[string]any {
	if len(sync) == 0 {
		return deferred
	}
	if len(deferred) == 0 {
		return sync
	}
	merged := make(map[string]any, len(sync)+len(deferred))
	for k, v := range sync {
		merged[k] = v
	}
	for k, v := range deferred {
		merged[k] = v
	}
	return merged
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
