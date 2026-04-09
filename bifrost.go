package bifrost

import (
	"embed"

	"github.com/3-lines-studio/bifrost/internal/app"
	"github.com/3-lines-studio/bifrost/internal/core"
)

type RedirectError = core.RedirectError

type StaticPathData = core.StaticPathData

type PageOption = core.PageOption

type Framework = core.Framework

const (
	React = core.FrameworkReact
)

type Route = core.Route

type ConfigOption = core.ConfigOption

func WithFramework(fw core.Framework) ConfigOption {
	return core.WithFramework(fw)
}

type App = app.App

func New(assetsFS embed.FS, routes ...Route) *App {
	return app.New(assetsFS, routes...)
}

func NewWithFramework(assetsFS embed.FS, fw Framework, routes ...Route) *App {
	return app.NewWithFramework(assetsFS, fw, routes...)
}

func NewWithOptions(assetsFS embed.FS, opts []ConfigOption, routes ...Route) *App {
	return app.NewWithOptions(assetsFS, opts, routes...)
}

func Page(pattern string, componentPath string, opts ...PageOption) Route {
	return core.Page(pattern, componentPath, opts...)
}

func WithLoader(loader core.PropsLoader) PageOption {
	return core.WithLoader(loader)
}

func WithDeferredLoader(loader core.DeferredPropsLoader) PageOption {
	return core.WithDeferredLoader(loader)
}

func WithClient() PageOption {
	return core.WithClient()
}

func WithStatic() PageOption {
	return core.WithStatic()
}

func WithStaticData(loader core.StaticDataLoader) PageOption {
	return core.WithStaticData(loader)
}

const PropHTMLLang = core.PropHTMLLang

const PropHTMLClass = core.PropHTMLClass

func WithDefaultHTMLLang(lang string) ConfigOption {
	return core.WithDefaultHTMLLang(lang)
}

func WithHTMLLang(lang string) PageOption {
	return core.WithHTMLLang(lang)
}

func WithHTMLClass(class string) PageOption {
	return core.WithHTMLClass(class)
}
