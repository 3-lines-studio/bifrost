package bifrost

import (
	"embed"
	"net/http"

	internalapp "github.com/3-lines-studio/bifrost/internal/app"
	"github.com/3-lines-studio/bifrost/internal/core"
)

// RedirectError describes an HTTP redirect returned by a props loader.
type RedirectError = core.RedirectError

// StaticPathData describes one path and its React props for static generation.
type StaticPathData = core.StaticPathData

// StaticDataLoader returns paths and props to generate at build time.
type StaticDataLoader = core.StaticDataLoader

// PropsLoader loads React props from an HTTP request.
type PropsLoader = core.PropsLoader

// PageOption configures one page.
type PageOption = core.PageOption

// Route describes one page route.
type Route = core.Route

// ConfigOption configures an App.
type ConfigOption = core.ConfigOption

// Router is the minimum router contract needed by App.Wrap.
type Router interface {
	http.Handler
	Handle(pattern string, handler http.Handler)
}

// App owns Bifrost routes and the configured JavaScript renderer, when one is needed.
type App struct {
	inner *internalapp.App
}

// New creates an App with the given routes.
func New(assetsFS embed.FS, routes ...Route) (*App, error) {
	inner, err := internalapp.New(assetsFS, routes...)
	if err != nil {
		return nil, err
	}
	return &App{inner: inner}, nil
}

// NewWithOptions creates an App with app-wide options and routes.
func NewWithOptions(assetsFS embed.FS, opts []ConfigOption, routes ...Route) (*App, error) {
	inner, err := internalapp.NewWithOptions(assetsFS, opts, routes...)
	if err != nil {
		return nil, err
	}
	return &App{inner: inner}, nil
}

// Handle adds routes before Handler or Wrap is called.
func (a *App) Handle(routes ...Route) error {
	return a.inner.Handle(routes...)
}

// Wrap registers Bifrost pages on router and adds asset handling.
func (a *App) Wrap(router Router) http.Handler {
	return a.inner.Wrap(router)
}

// Handler returns an HTTP handler for Bifrost pages and assets.
func (a *App) Handler() http.Handler {
	return a.inner.Handler()
}

// Stop stops the JavaScript renderer and removes temporary files.
func (a *App) Stop() error {
	return a.inner.Stop()
}

// ExportStaticPages renders static pages into outputDir.
func (a *App) ExportStaticPages(outputDir string) error {
	return a.inner.ExportStaticPages(outputDir)
}

// Page creates a React page route.
func Page(pattern string, componentPath string, opts ...PageOption) Route {
	return core.Page(pattern, componentPath, opts...)
}

// WithLoader sets the props loader for an SSR page.
func WithLoader(loader PropsLoader) PageOption {
	return core.WithLoader(loader)
}

// PreLoaderResult carries the document attributes a pre loader decides before
// the props loader runs.
type PreLoaderResult = core.PreLoaderResult

// PreLoader runs before the response starts and decides redirects and document
// attributes for an SSR page.
type PreLoader = core.PreLoader

// WithPreLoader sets the pre loader for an SSR page. It runs before the
// response starts: return a RedirectError to redirect with a real status code,
// or set Lang/Class to control the document attributes in the streamed head.
func WithPreLoader(loader PreLoader) PageOption {
	return core.WithPreLoader(loader)
}

// PreloadKind describes the kind of asset hint a page declares.
type PreloadKind = core.PreloadKind

const (
	PreloadLink PreloadKind = core.PreloadLink
	Preconnect  PreloadKind = core.Preconnect
	DNSPrefetch PreloadKind = core.DNSPrefetch
)

// Preload declares an asset hint for the streamed document head.
type Preload = core.Preload

// WithPreloads declares asset hints for an SSR page. They are written into the
// streamed head and the 103 Early Hints response.
func WithPreloads(preloads ...Preload) PageOption {
	return core.WithPreloads(preloads...)
}

// WithStreamingShell sets a CSS fragment rendered as a full-viewport loading
// overlay in the streamed head while the page renders. The overlay is removed
// automatically once content mounts into <div id="app">.
func WithStreamingShell(css string) PageOption {
	return core.WithStreamingShell(css)
}

// WithPrerender declares likely next navigations for an SSR page. Bifrost
// writes a speculationrules script into the streamed head so Chromium
// prerenders them.
func WithPrerender(paths ...string) PageOption {
	return core.WithPrerender(paths...)
}

// WithClient makes a page render only in the browser.
func WithClient() PageOption {
	return core.WithClient()
}

// WithStatic makes a page render to static HTML at build time.
func WithStatic() PageOption {
	return core.WithStatic()
}

// WithStaticData makes a page render the loader's paths at build time.
func WithStaticData(loader StaticDataLoader) PageOption {
	return core.WithStaticData(loader)
}

// WithDefaultHTMLLang sets the app's default document language.
func WithDefaultHTMLLang(lang string) ConfigOption {
	return core.WithDefaultHTMLLang(lang)
}

// WithHTMLLang sets a page's document language.
func WithHTMLLang(lang string) PageOption {
	return core.WithHTMLLang(lang)
}

// WithHTMLClass sets a page's document class.
func WithHTMLClass(class string) PageOption {
	return core.WithHTMLClass(class)
}
