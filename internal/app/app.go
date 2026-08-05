package app

import (
	"embed"
	"fmt"
	"net/http"
	"os"

	"github.com/3-lines-studio/bifrost/internal/adapters/env"
	"github.com/3-lines-studio/bifrost/internal/adapters/framework"
	adaptersfs "github.com/3-lines-studio/bifrost/internal/adapters/fs"
	adaptershttp "github.com/3-lines-studio/bifrost/internal/adapters/http"
	"github.com/3-lines-studio/bifrost/internal/adapters/runtime"
	"github.com/3-lines-studio/bifrost/internal/core"
	"github.com/3-lines-studio/bifrost/internal/usecase"
)

type Router interface {
	http.Handler
	Handle(pattern string, handler http.Handler)
}

type App struct {
	host         *runtime.Host
	routes       []core.Route
	routeConfigs []core.PageConfig
	assetsFS     embed.FS
	isDev        bool
	manifest     *core.Manifest
	pageConfigs  map[string]*core.PageConfig
	config       *core.Config
	adapter      core.FrameworkAdapter
	routesSealed bool
}

func New(assetsFS embed.FS, routes ...core.Route) (*App, error) {
	return newApp(assetsFS, routes, &core.Config{})
}

func NewWithOptions(assetsFS embed.FS, opts []core.ConfigOption, routes ...core.Route) (*App, error) {
	config := &core.Config{}
	for _, o := range opts {
		o(config)
	}
	return newApp(assetsFS, routes, config)
}

func newApp(assetsFS embed.FS, routes []core.Route, config *core.Config) (*App, error) {
	mode := env.DetectAppMode()
	app := &App{
		assetsFS:    assetsFS,
		isDev:       mode == core.ModeDev,
		pageConfigs: make(map[string]*core.PageConfig),
		config:      config,
		adapter:     framework.DefaultAdapter(),
	}
	if err := app.addRoutes(routes); err != nil {
		return nil, err
	}

	if env.IsExportMarkerPresent() || mode == core.ModeExport {
		return app, nil
	}

	h, err := runtime.NewHost(assetsFS, mode, app.adapter)
	if err != nil {
		return nil, fmt.Errorf("failed to create bifrost renderer: %w", err)
	}
	app.host = h
	app.manifest = h.Manifest()

	return app, nil
}

func (a *App) addRoutes(routes []core.Route) error {
	modes := make(map[string]core.PageMode, len(a.pageConfigs)+len(routes))
	for componentPath, config := range a.pageConfigs {
		modes[componentPath] = config.Mode
	}

	storedRoutes := make([]core.Route, len(routes))
	configs := make([]core.PageConfig, len(routes))
	for i, route := range routes {
		route.Options = append([]core.PageOption(nil), route.Options...)
		pc, err := core.PageConfigFromRoute(route)
		if err != nil {
			return err
		}
		if previous, ok := modes[route.ComponentPath]; ok && previous != pc.Mode {
			return fmt.Errorf(
				"bifrost: component %q cannot use both %s and %s modes",
				route.ComponentPath,
				previous.BuildLabel(),
				pc.Mode.BuildLabel(),
			)
		}
		modes[route.ComponentPath] = pc.Mode
		storedRoutes[i] = route
		configs[i] = pc
	}

	for i := range storedRoutes {
		config := configs[i]
		a.pageConfigs[storedRoutes[i].ComponentPath] = &config
	}
	a.routes = append(a.routes, storedRoutes...)
	a.routeConfigs = append(a.routeConfigs, configs...)
	return nil
}

func (a *App) Handle(routes ...core.Route) error {
	if a.routesSealed {
		return fmt.Errorf("bifrost: Handle after Wrap or Handler")
	}
	return a.addRoutes(routes)
}

func (a *App) runExportMode() {
	h, err := runtime.NewHost(a.assetsFS, core.ModeExport, a.adapter)
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

func (a *App) Wrap(api Router) http.Handler {
	if env.IsExportMarkerPresent() {
		if err := usecase.WriteStaticBuildExportToStdout(a.routes); err != nil {
			fmt.Fprintf(os.Stderr, "export failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if env.DetectAppMode() == core.ModeExport {
		a.runExportMode()
	}

	if api == nil {
		panic("bifrost: nil router passed to Wrap; use app.Handler()")
	}

	a.routesSealed = true

	defaultLang := ""
	if a.config != nil {
		defaultLang = a.config.DefaultHTMLLang
	}

	fsAdapter := adaptersfs.NewEmbedFileSystem(a.assetsFS)
	pageService := usecase.NewPageService(a.host.Client(), fsAdapter, a.adapter)

	for i, route := range a.routes {
		config := a.routeConfigs[i]
		staticPath := a.getStaticPath(config)

		handler := adaptershttp.NewPageHandler(pageService, config, a.manifest, a.assetsFS, a.isDev, staticPath, defaultLang)
		api.Handle(route.Pattern, handler)
	}

	if shouldPrintRouteTable() {
		printRouteTable(a.routes, a.routeConfigs)
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
		if !a.isDev && a.host != nil && entry.SSR != "" {
			return a.host.ResolveSSRBundlePath(entry.SSR)
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
	if a.host != nil {
		return a.host.ResolveSSRBundlePath(entry.SSR)
	}
	return entry.SSR
}

func (a *App) Stop() error {
	if a.host != nil {
		return a.host.Stop()
	}
	return nil
}

func (a *App) ExportStaticPages(outputDir string) error {
	var r usecase.Renderer
	if a.host != nil {
		r = a.host.Client()
	}
	return usecase.ExportStaticPages(usecase.ExportStaticPagesInput{
		OutputDir:    outputDir,
		Routes:       a.routes,
		Manifest:     a.manifest,
		AppConfig:    a.config,
		SSBundlePath: a.getSSBundlePath,
		Renderer:     r,
	})
}

func createAssetHandler(router Router, app *App) http.Handler {
	isDev := app.isDev
	assetHandler := adaptershttp.NewAssetHandler(app.assetsFS, isDev)
	mdRouter := adaptershttp.ResolveMarkdown(router)

	distHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		path := req.URL.Path

		if len(path) >= 6 && path[:6] == "/dist/" {
			assetHandler.ServeHTTP(w, req)
			return
		}

		mdRouter.ServeHTTP(w, req)
	})

	return adaptershttp.NewPublicHandler(app.assetsFS, distHandler, isDev)
}
