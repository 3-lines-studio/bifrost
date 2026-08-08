package core

import (
	"context"
	"net/http"
)

type PropsLoader func(*http.Request) (any, error)

type PreLoader func(*http.Request) (PreLoaderResult, error)

type PreLoaderResult struct {
	Lang     string
	Class    string
	Preloads []Preload
}

// PreloadKind describes the kind of asset hint a page declares for the
// streamed document head.
type PreloadKind int

const (
	PreloadLink PreloadKind = iota
	Preconnect
	DNSPrefetch
)

// Preload declares an asset hint written into the streamed head and the 103
// Early Hints response. Href is required; As and FetchPriority apply to
// PreloadLink.
type Preload struct {
	Kind          PreloadKind
	Href          string
	As            string
	FetchPriority string
}

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
	PreLoader        PreLoader
	StaticDataLoader StaticDataLoader
	Preloads         []Preload
	StreamingShell   string
	PrerenderPaths   []string
	HTMLLang         string
	HTMLClass        string
	modeOptions      uint8
	optionFlags      uint8
}

const (
	optionLoader uint8 = 1 << iota
	optionPreLoader
	optionStatic
	optionStaticData
	optionPreloads
	optionStreamingShell
	optionPrerender
)

type PageOption func(*PageConfig)

func WithLoader(loader PropsLoader) PageOption {
	return func(c *PageConfig) {
		c.PropsLoader = loader
		c.optionFlags |= optionLoader
	}
}

// WithPreLoader sets the pre loader for an SSR page. It runs before the
// response starts: return a RedirectError to redirect with a real status code,
// or set Lang/Class to control the document attributes in the streamed head.
func WithPreLoader(loader PreLoader) PageOption {
	return func(c *PageConfig) {
		c.PreLoader = loader
		c.optionFlags |= optionPreLoader
	}
}

// WithPreloads declares asset hints for an SSR page. They are written into the
// streamed head and the 103 Early Hints response. The pre loader can add
// request-dependent hints via PreLoaderResult.Preloads.
func WithPreloads(preloads ...Preload) PageOption {
	return func(c *PageConfig) {
		c.Preloads = append(c.Preloads, preloads...)
		c.optionFlags |= optionPreloads
	}
}

// WithStreamingShell sets a CSS fragment rendered as a full-viewport loading
// overlay in the streamed head while the page renders. The overlay is removed
// automatically once content mounts into <div id="app">.
func WithStreamingShell(css string) PageOption {
	return func(c *PageConfig) {
		c.StreamingShell = css
		c.optionFlags |= optionStreamingShell
	}
}

// WithPrerender declares likely next navigations for an SSR page. Bifrost
// writes a speculationrules script into the streamed head so Chromium
// prerenders them.
func WithPrerender(paths ...string) PageOption {
	return func(c *PageConfig) {
		c.PrerenderPaths = append(c.PrerenderPaths, paths...)
		c.optionFlags |= optionPrerender
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
