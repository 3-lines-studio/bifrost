package bifrost

import (
	"embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/3-lines-studio/bifrost/internal/adapters/framework"
	adaptersfs "github.com/3-lines-studio/bifrost/internal/adapters/fs"
	adaptershttp "github.com/3-lines-studio/bifrost/internal/adapters/http"
	"github.com/3-lines-studio/bifrost/internal/core"
	"github.com/3-lines-studio/bifrost/internal/usecase"
)

const exportMarkerPath = ".bifrost/.export-mode"

func detectMode() core.Mode {
	if os.Getenv("BIFROST_EXPORT") == "1" {
		return core.ModeExport
	}
	if os.Getenv("BIFROST_DEV") == "1" {
		return core.ModeDev
	}
	return core.ModeProd
}

func isExportMode() bool {
	_, err := os.Stat(exportMarkerPath)
	return err == nil
}

type RedirectError = core.RedirectError

type StaticPathData = core.StaticPathData

type PageOption = core.PageOption

type Framework = core.Framework

const (
	React = core.FrameworkReact
)

type Route struct {
	Pattern       string
	ComponentPath string
	Options       []PageOption
}

func buildPageConfig(route Route) core.PageConfig {
	config := core.PageConfig{
		ComponentPath: route.ComponentPath,
		Mode:          core.ModeSSR,
	}
	for _, opt := range route.Options {
		opt(&config)
	}
	return config
}

type ConfigOption = core.ConfigOption

func WithFramework(fw core.Framework) ConfigOption {
	return core.WithFramework(fw)
}

type App struct {
	renderer    *renderer
	routes      []Route
	assetsFS    embed.FS
	isDev       bool
	manifest    *core.Manifest
	pageConfigs map[string]*core.PageConfig
	config      *core.Config
}

type router interface {
	http.Handler
	Handle(pattern string, handler http.Handler)
}

func newApp(assetsFS embed.FS, routes []Route, config *core.Config) *App {
	mode := detectMode()
	app := &App{
		assetsFS:    assetsFS,
		isDev:       mode == core.ModeDev,
		routes:      routes,
		pageConfigs: make(map[string]*core.PageConfig),
		config:      config,
	}

	for _, route := range routes {
		config := buildPageConfig(route)
		app.pageConfigs[route.ComponentPath] = &config
	}

	if isExportMode() {
		return app
	}

	if mode == core.ModeExport {
		app.runExportMode()
	}

	r, err := newRenderer(assetsFS, mode, app.getAdapter())
	if err != nil {
		panic(fmt.Sprintf("failed to create bifrost renderer: %v", err))
	}
	app.renderer = r
	app.manifest = r.manifest

	return app
}

func New(assetsFS embed.FS, routes ...Route) *App {
	config := &core.Config{
		Framework: core.FrameworkReact,
	}
	return newApp(assetsFS, routes, config)
}

func NewWithFramework(assetsFS embed.FS, fw core.Framework, routes ...Route) *App {
	config := &core.Config{
		Framework: fw,
	}
	return newApp(assetsFS, routes, config)
}

// NewWithOptions constructs an app and applies ConfigOption values (e.g. WithDefaultHTMLLang).
func NewWithOptions(assetsFS embed.FS, opts []ConfigOption, routes ...Route) *App {
	config := &core.Config{
		Framework: core.FrameworkReact,
	}
	for _, o := range opts {
		o(config)
	}
	return newApp(assetsFS, routes, config)
}

func (a *App) getAdapter() core.FrameworkAdapter {
	return framework.NewReactAdapter()
}

func (a *App) runExportMode() {
	r, err := newRenderer(a.assetsFS, core.ModeExport, a.getAdapter())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Export failed: %v\n", err)
		os.Exit(1)
	}
	a.renderer = r
	a.manifest = r.manifest

	outputDir := os.Getenv("BIFROST_EXPORT_DIR")
	if outputDir == "" {
		outputDir = ".bifrost"
	}

	if err := a.ExportStaticPages(outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Export failed: %v\n", err)
		os.Exit(1)
	}

	_ = a.Stop()
	os.Exit(0)
}

func (a *App) Wrap(api router) http.Handler {
	if isExportMode() {
		if err := exportStaticBuildData(a); err != nil {
			fmt.Fprintf(os.Stderr, "export failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if api == nil {
		panic("bifrost: nil router passed to Wrap; use app.Handler()")
	}

	defaultLang := ""
	if a.config != nil {
		defaultLang = a.config.DefaultHTMLLang
	}

	for _, route := range a.routes {
		config := buildPageConfig(route)
		staticPath := a.getStaticPath(config)

		fsAdapter := adaptersfs.NewEmbedFileSystem(a.assetsFS)
		pageService := usecase.NewPageService(a.renderer.client, fsAdapter, a.getAdapter())

		handler := adaptershttp.NewPageHandler(pageService, config, a.manifest, a.assetsFS, a.isDev, staticPath, defaultLang)
		api.Handle(route.Pattern, handler)
	}

	return createAssetHandler(api, a)
}

func (a *App) Handler() http.Handler {
	return a.Wrap(http.NewServeMux())
}

func (a *App) getStaticPath(config core.PageConfig) string {
	if a.manifest == nil {
		return ""
	}
	entryName := core.EntryNameForPath(config.ComponentPath)
	entry, ok := a.manifest.Entries[entryName]
	if !ok {
		return ""
	}

	switch config.Mode {
	case core.ModeClientOnly:
		return entry.HTML
	default:
		if !a.isDev && a.renderer != nil && a.renderer.ssrTempDir != "" && entry.SSR != "" {
			return filepath.Join(a.renderer.ssrTempDir, entry.SSR)
		}
		return entry.SSR
	}
}

func (a *App) getSSBundlePath(entryName string) string {
	if a.manifest == nil {
		return ""
	}
	entry, ok := a.manifest.Entries[entryName]
	if !ok || entry.SSR == "" {
		return ""
	}
	if a.renderer != nil && a.renderer.ssrTempDir != "" {
		return filepath.Join(a.renderer.ssrTempDir, entry.SSR)
	}
	return entry.SSR
}

func (a *App) Stop() error {
	if a.renderer != nil {
		return a.renderer.stop()
	}
	return nil
}

func Page(pattern string, componentPath string, opts ...PageOption) Route {
	return Route{
		Pattern:       pattern,
		ComponentPath: componentPath,
		Options:       opts,
	}
}

func WithLoader(loader core.PropsLoader) PageOption {
	return core.WithLoader(loader)
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

// PropHTMLLang is the reserved loader/static-data key for document language (see WithHTMLLang / WithDefaultHTMLLang).
const PropHTMLLang = core.PropHTMLLang

// PropHTMLClass is the reserved loader/static-data key for document class (see WithHTMLClass).
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

func createAssetHandler(router router, app *App) http.Handler {
	isDev := app.isDev

	distHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		path := req.URL.Path

		if len(path) >= 6 && path[:6] == "/dist/" {
			assetHandler := adaptershttp.NewAssetHandler(app.assetsFS, isDev)
			assetHandler.ServeHTTP(w, req)
			return
		}

		router.ServeHTTP(w, req)
	})

	return adaptershttp.NewPublicHandler(app.assetsFS, distHandler, isDev)
}
