package bifrost

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

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

// ExportStaticPages generates static HTML files for StaticPrerender pages
func (a *App) ExportStaticPages(outputDir string) error {
	pagesDir := filepath.Join(outputDir, "pages", "routes")
	if err := os.MkdirAll(pagesDir, 0755); err != nil {
		return fmt.Errorf("failed to create pages directory: %w", err)
	}

	// Build manifest with StaticRoutes
	exportManifest := &core.Manifest{
		Entries: make(map[string]core.ManifestEntry),
	}

	for _, route := range a.routes {
		config := buildPageConfig(route)
		if config.Mode != core.ModeStaticPrerender {
			continue
		}

		entryName := core.EntryNameForPath(config.ComponentPath)
		ssrBundlePath := a.getSSBundlePath(entryName)
		if ssrBundlePath == "" {
			fmt.Printf("Warning: No SSR bundle for %s, skipping\n", route.Pattern)
			continue
		}

		var entries []core.StaticPathData
		if config.StaticDataLoader != nil {
			var err error
			entries, err = config.StaticDataLoader(context.Background())
			if err != nil {
				fmt.Printf("Warning: Failed to load static data for %s: %v, skipping\n", route.Pattern, err)
				continue
			}
		} else {
			// Static page without data loader - create single entry with empty props
			entries = []core.StaticPathData{
				{
					Path:  route.Pattern,
					Props: map[string]any{},
				},
			}
		}

		manifestEntry := core.ManifestEntry{
			Script:       a.manifest.Entries[entryName].Script,
			CSS:          a.manifest.Entries[entryName].CSS,
			Chunks:       a.manifest.Entries[entryName].Chunks,
			Mode:         "static",
			StaticRoutes: make(map[string]string),
		}

		for _, entry := range entries {
			fmt.Printf("Exporting %s...\n", entry.Path)

			appDefault := ""
			if a.config != nil {
				appDefault = a.config.DefaultHTMLLang
			}
			lang, propsForReact := core.ResolveHTMLLang(appDefault, config.HTMLLang, entry.Props)

			page, err := a.renderer.client.Render(ssrBundlePath, propsForReact)
			if err != nil {
				fmt.Printf("Warning: Failed to render %s: %v, skipping\n", entry.Path, err)
				continue
			}

			html, err := core.RenderHTMLShell(page.Body, propsForReact, manifestEntry.Script, page.Head, manifestEntry.CSS, manifestEntry.Chunks, lang)
			if err != nil {
				fmt.Printf("Warning: Failed to build HTML for %s: %v, skipping\n", entry.Path, err)
				continue
			}

			cleanedRoutePath := path.Clean("/" + entry.Path)
			if strings.Contains(cleanedRoutePath, "..") {
				fmt.Printf("Warning: Unsafe route path %s, skipping\n", entry.Path)
				continue
			}

			htmlPath := filepath.Join(pagesDir, filepath.FromSlash(cleanedRoutePath), "index.html")
			absHTML, err := filepath.Abs(htmlPath)
			if err != nil {
				fmt.Printf("Warning: Failed to resolve path for %s: %v, skipping\n", entry.Path, err)
				continue
			}
			absPages, err := filepath.Abs(pagesDir)
			if err != nil {
				fmt.Printf("Warning: Failed to resolve pages dir: %v, skipping\n", err)
				continue
			}
			if !strings.HasPrefix(absHTML, absPages+string(filepath.Separator)) {
				fmt.Printf("Warning: Route path %s escapes output directory, skipping\n", entry.Path)
				continue
			}

			if err := os.MkdirAll(filepath.Dir(htmlPath), 0755); err != nil {
				fmt.Printf("Warning: Failed to create directory for %s: %v, skipping\n", entry.Path, err)
				continue
			}

			if err := os.WriteFile(htmlPath, []byte(html), 0644); err != nil {
				fmt.Printf("Warning: Failed to write %s: %v, skipping\n", entry.Path, err)
				continue
			}

			normalizedPath := core.NormalizePath(entry.Path)
			manifestEntry.StaticRoutes[normalizedPath] = "/pages/routes" + cleanedRoutePath + "/index.html"
		}

		exportManifest.Entries[entryName] = manifestEntry
	}

	// Write export manifest
	manifestData, err := json.MarshalIndent(exportManifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal export manifest: %w", err)
	}

	manifestPath := filepath.Join(outputDir, "export-manifest.json")
	return os.WriteFile(manifestPath, manifestData, 0644)
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

func WithDefaultHTMLLang(lang string) ConfigOption {
	return core.WithDefaultHTMLLang(lang)
}

func WithHTMLLang(lang string) PageOption {
	return core.WithHTMLLang(lang)
}

func WithSuppressHydrationWarningRoot() PageOption {
	return core.WithSuppressHydrationWarningRoot()
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
