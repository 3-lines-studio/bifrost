package core

import (
	"context"
	"net/http"
)

type PropsLoader func(*http.Request) (any, error)

type RedirectError interface {
	error
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
	Props any
}

type StaticDataLoader func(context.Context) ([]StaticPathData, error)

type PageConfig struct {
	ComponentPath    string
	Mode             PageMode
	PropsLoader      PropsLoader
	StaticDataLoader StaticDataLoader
	HTMLLang         string
	HTMLClass        string
	modeOptions      uint8
	optionFlags      uint8
}

const (
	optionLoader uint8 = 1 << iota
	optionStatic
	optionStaticData
)

type PageOption func(*PageConfig)

func WithLoader(loader PropsLoader) PageOption {
	return func(c *PageConfig) {
		c.PropsLoader = loader
		c.optionFlags |= optionLoader
	}
}

func WithClient() PageOption {
	return func(c *PageConfig) {
		c.Mode = ModeClientOnly
		c.modeOptions |= 1
	}
}

func WithStatic() PageOption {
	return func(c *PageConfig) {
		c.Mode = ModeStaticPrerender
		c.modeOptions |= 2
		c.optionFlags |= optionStatic
	}
}

func WithStaticData(loader StaticDataLoader) PageOption {
	return func(c *PageConfig) {
		c.Mode = ModeStaticPrerender
		c.StaticDataLoader = loader
		c.modeOptions |= 2
		c.optionFlags |= optionStaticData
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

func isNilOrEmptyProps(p any) bool {
	if p == nil {
		return true
	}
	m, ok := p.(map[string]any)
	return ok && len(m) == 0
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

type Config struct {
	DefaultHTMLLang string
}

type ConfigOption func(*Config)

func WithDefaultHTMLLang(lang string) ConfigOption {
	return func(c *Config) {
		c.DefaultHTMLLang = lang
	}
}
