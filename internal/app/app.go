package app

import (
	"embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/3-lines-studio/bifrost/internal/adapters/env"
	"github.com/3-lines-studio/bifrost/internal/adapters/framework"
	adaptersfs "github.com/3-lines-studio/bifrost/internal/adapters/fs"
	adaptershttp "github.com/3-lines-studio/bifrost/internal/adapters/http"
	"github.com/3-lines-studio/bifrost/internal/adapters/runtime"
	"github.com/3-lines-studio/bifrost/internal/core"
	"github.com/3-lines-studio/bifrost/internal/usecase"
)

// Router is the minimal API Bifrost needs from an HTTP mux (e.g. *http.ServeMux).
type Router interface {
	http.Handler
	Handle(pattern string, handler http.Handler)
}

// App is the Bifrost application (routes, assets, SSR runtime).
type App struct {
	host        *runtime.Host
	routes      []core.Route
	assetsFS    embed.FS
	isDev       bool
	manifest    *core.Manifest
	pageConfigs map[string]*core.PageConfig
	config      *core.Config
}

// New constructs an app with default React framework.
func New(assetsFS embed.FS, routes ...core.Route) *App {
	config := &core.Config{
		Framework: core.FrameworkReact,
	}
	return newApp(assetsFS, routes, config)
}

// NewWithFramework constructs an app with the given framework constant.
func NewWithFramework(assetsFS embed.FS, fw core.Framework, routes ...core.Route) *App {
	config := &core.Config{
		Framework: fw,
	}
	return newApp(assetsFS, routes, config)
}

// NewWithOptions constructs an app and applies ConfigOption values (e.g. WithDefaultHTMLLang).
func NewWithOptions(assetsFS embed.FS, opts []core.ConfigOption, routes ...core.Route) *App {
	config := &core.Config{
		Framework: core.FrameworkReact,
	}
	for _, o := range opts {
		o(config)
	}
	return newApp(assetsFS, routes, config)
}

func newApp(assetsFS embed.FS, routes []core.Route, config *core.Config) *App {
	mode := env.DetectAppMode()
	app := &App{
		assetsFS:    assetsFS,
		isDev:       mode == core.ModeDev,
		routes:      routes,
		pageConfigs: make(map[string]*core.PageConfig),
		config:      config,
	}

	for _, route := range routes {
		pc := core.PageConfigFromRoute(route)
		app.pageConfigs[route.ComponentPath] = &pc
	}

	if env.IsExportMarkerPresent() {
		return app
	}

	if mode == core.ModeExport {
		app.runExportMode()
	}

	h, err := runtime.NewHost(assetsFS, mode, app.getAdapter())
	if err != nil {
		panic(fmt.Sprintf("failed to create bifrost renderer: %v", err))
	}
	app.host = h
	app.manifest = h.Manifest()

	return app
}

func (a *App) getAdapter() core.FrameworkAdapter {
	return framework.NewReactAdapter()
}

func (a *App) runExportMode() {
	h, err := runtime.NewHost(a.assetsFS, core.ModeExport, a.getAdapter())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Export failed: %v\n", err)
		os.Exit(1)
	}
	a.host = h
	a.manifest = h.Manifest()

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

// Wrap mounts page handlers on the given router and returns the full HTTP handler.
func (a *App) Wrap(api Router) http.Handler {
	if env.IsExportMarkerPresent() {
		if err := usecase.WriteStaticBuildExportToStdout(a.routes, a.pageConfigs); err != nil {
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
		config := core.PageConfigFromRoute(route)
		staticPath := a.getStaticPath(config)

		fsAdapter := adaptersfs.NewEmbedFileSystem(a.assetsFS)
		pageService := usecase.NewPageService(a.host.Client(), fsAdapter, a.getAdapter())

		handler := adaptershttp.NewPageHandler(pageService, config, a.manifest, a.assetsFS, a.isDev, staticPath, defaultLang)
		api.Handle(route.Pattern, handler)
	}

	return createAssetHandler(api, a)
}

// Handler returns a mux with all Bifrost routes registered.
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
		if !a.isDev && a.host != nil && a.host.SSRTempDir() != "" && entry.SSR != "" {
			return filepath.Join(a.host.SSRTempDir(), entry.SSR)
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
	if a.host != nil && a.host.SSRTempDir() != "" {
		return filepath.Join(a.host.SSRTempDir(), entry.SSR)
	}
	return entry.SSR
}

// Stop releases renderer resources.
func (a *App) Stop() error {
	if a.host != nil {
		return a.host.Stop()
	}
	return nil
}

// ExportStaticPages generates static HTML files for StaticPrerender pages.
func (a *App) ExportStaticPages(outputDir string) error {
	var r usecase.Renderer
	if a.host != nil {
		r = a.host.Client()
	}
	return usecase.ExportStaticPages(usecase.ExportStaticPagesInput{
		OutputDir:    outputDir,
		Routes:       a.routes,
		PageConfigs:  a.pageConfigs,
		Manifest:     a.manifest,
		AppConfig:    a.config,
		SSBundlePath: a.getSSBundlePath,
		Renderer:     r,
	})
}

func createAssetHandler(router Router, app *App) http.Handler {
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
