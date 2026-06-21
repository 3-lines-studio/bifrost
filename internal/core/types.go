package core

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
)

type PropsLoader func(*http.Request) (any, error)

type DeferredPropsLoader func(*http.Request) (any, error)

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
	Props any
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

func isNilOrEmptyProps(p any) bool {
	if p == nil {
		return true
	}
	m, ok := p.(map[string]any)
	return ok && len(m) == 0
}

func mergeMaps(sync, deferred map[string]any) map[string]any {
	merged := make(map[string]any, len(sync)+len(deferred))
	maps.Copy(merged, sync)
	maps.Copy(merged, deferred)
	return merged
}

func propsToMap(v any) (map[string]any, bool) {
	if m, ok := v.(map[string]any); ok {
		return m, true
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, false
	}
	return m, true
}

func MergeProps(sync any, deferred any) any {
	if isNilOrEmptyProps(sync) {
		return deferred
	}
	if isNilOrEmptyProps(deferred) {
		return sync
	}
	if sm, ok := sync.(map[string]any); ok {
		if dm, ok := deferred.(map[string]any); ok {
			return mergeMaps(sm, dm)
		}
	}
	syncMap, syncOK := propsToMap(sync)
	deferredMap, deferredOK := propsToMap(deferred)
	if !syncOK {
		return deferred
	}
	if !deferredOK {
		return sync
	}
	return mergeMaps(syncMap, deferredMap)
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
	Render(componentPath string, props any) (RenderedPage, error)
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
