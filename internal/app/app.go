package app

import (
	"embed"
	"fmt"
	"net/http"
	"os"

	"github.com/3-lines-studio/bifrost/internal/adapters/env"
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
	routesSealed bool
}

func New(assetsFS embed.FS, routes ...core.Route) (*App, error) {
	return newApp(assetsFS, routes, &core.Config{})
}

func NewWithOptions(assetsFS embed.FS, opts []core.ConfigOption, routes ...core.Route) (*App, error) {
	config := &core.Config{}
	for _, o := range opts {
		if o == nil {
			return nil, fmt.Errorf("bifrost: config option cannot be nil")
		}
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
	}
	if err := app.addRoutes(routes); err != nil {
		return nil, err
	}

	if mode == core.ModeExport {
		return app, nil
	}

	h, err := runtime.NewHost(assetsFS, mode)
	if err != nil {
		return nil, fmt.Errorf("failed to create bifrost renderer: %w", err)
	}
	app.host = h
	app.manifest = h.Manifest()
	if mode != core.ModeDev {
		if err := validateProductionManifest(app.routeConfigs, app.manifest); err != nil {
			_ = h.Stop()
			return nil, fmt.Errorf("bifrost: invalid build assets: %w", err)
		}
	}

	return app, nil
}

func (a *App) addRoutes(routes []core.Route) error {
	modes := make(map[string]core.PageMode, len(a.pageConfigs)+len(routes))
	clientDocumentAttrs := make(map[string][2]string)
	patterns := make(map[string]struct{}, len(a.routes)+len(routes))
	entryComponents := make(map[string]string, len(a.routes)+len(routes))
	for componentPath, config := range a.pageConfigs {
		modes[componentPath] = config.Mode
		if config.Mode == core.ModeClientOnly {
			clientDocumentAttrs[componentPath] = [2]string{config.HTMLLang, config.HTMLClass}
		}
	}
	for _, route := range a.routes {
		patterns[route.Pattern] = struct{}{}
		entryComponents[core.EntryNameForPath(route.ComponentPath)] = route.ComponentPath
	}

	storedRoutes := make([]core.Route, len(routes))
	configs := make([]core.PageConfig, len(routes))
	for i, route := range routes {
		route.Options = append([]core.PageOption(nil), route.Options...)
		pc, err := core.PageConfigFromRoute(route)
		if err != nil {
			return err
		}
		if _, exists := patterns[route.Pattern]; exists {
			return fmt.Errorf("bifrost: duplicate page pattern %q", route.Pattern)
		}
		patterns[route.Pattern] = struct{}{}

		entryName := core.EntryNameForPath(route.ComponentPath)
		if previous, exists := entryComponents[entryName]; exists && previous != route.ComponentPath {
			return fmt.Errorf(
				"bifrost: components %q and %q map to the same build entry %q",
				previous,
				route.ComponentPath,
				entryName,
			)
		}
		entryComponents[entryName] = route.ComponentPath

		if previous, ok := modes[route.ComponentPath]; ok && previous != pc.Mode {
			return fmt.Errorf(
				"bifrost: component %q cannot use both %s and %s modes",
				route.ComponentPath,
				previous.BuildLabel(),
				pc.Mode.BuildLabel(),
			)
		}
		if pc.Mode == core.ModeClientOnly {
			attrs := [2]string{pc.HTMLLang, pc.HTMLClass}
			if previous, ok := clientDocumentAttrs[route.ComponentPath]; ok && previous != attrs {
				return fmt.Errorf(
					"bifrost: client-only component %q cannot use different HTML attributes across routes",
					route.ComponentPath,
				)
			}
			clientDocumentAttrs[route.ComponentPath] = attrs
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
	if !a.isDev && a.manifest != nil {
		configs := make([]core.PageConfig, len(routes))
		for i, route := range routes {
			config, err := core.PageConfigFromRoute(route)
			if err != nil {
				return err
			}
			configs[i] = config
		}
		if err := validateProductionManifest(configs, a.manifest); err != nil {
			return fmt.Errorf("bifrost: invalid build assets: %w", err)
		}
	}
	return a.addRoutes(routes)
}

func (a *App) runExportMode() {
	h, err := runtime.NewHost(a.assetsFS, core.ModeExport)
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
		_ = a.Stop()
		os.Exit(1)
	}

	_ = a.Stop()
	os.Exit(0)
}

func (a *App) Wrap(api Router) http.Handler {
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

	pageService := usecase.NewPageService(a.host.Client())

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
