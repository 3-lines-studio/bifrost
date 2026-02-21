package bifrost

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

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

type App struct {
	renderer    *renderer
	routes      []Route
	assetsFS    embed.FS
	isDev       bool
	manifest    *core.Manifest
	pageConfigs map[string]*core.PageConfig
}

type router interface {
	http.Handler
	Handle(pattern string, handler http.Handler)
}

func New(assetsFS embed.FS, routes ...Route) *App {
	mode := detectMode()
	app := &App{
		assetsFS:    assetsFS,
		isDev:       mode == core.ModeDev,
		routes:      routes,
		pageConfigs: make(map[string]*core.PageConfig),
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

	r, err := newRenderer(assetsFS, mode)
	if err != nil {
		panic(fmt.Sprintf("failed to create bifrost renderer: %v", err))
	}
	app.renderer = r
	app.manifest = r.manifest

	return app
}

func (a *App) runExportMode() {
	r, err := newRenderer(a.assetsFS, core.ModeExport)
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

	for _, route := range a.routes {
		config := buildPageConfig(route)
		staticPath := a.getStaticPath(config)

		fsAdapter := adaptersfs.NewEmbedFileSystem(a.assetsFS)
		pageService := usecase.NewPageService(a.renderer.client, fsAdapter)

		handler := adaptershttp.NewPageHandler(pageService, config, a.manifest, a.assetsFS, a.isDev, staticPath)
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
			Mode:         "static",
			StaticRoutes: make(map[string]string),
		}

		for _, entry := range entries {
			fmt.Printf("Exporting %s...\n", entry.Path)

			page, err := a.renderer.client.Render(ssrBundlePath, entry.Props)
			if err != nil {
				fmt.Printf("Warning: Failed to render %s: %v, skipping\n", entry.Path, err)
				continue
			}

			html := renderFullHTML(page, entryName, entry.Props)
			htmlPath := filepath.Join(pagesDir, entry.Path, "index.html")

			if err := os.MkdirAll(filepath.Dir(htmlPath), 0755); err != nil {
				fmt.Printf("Warning: Failed to create directory for %s: %v, skipping\n", entry.Path, err)
				continue
			}

			if err := os.WriteFile(htmlPath, []byte(html), 0644); err != nil {
				fmt.Printf("Warning: Failed to write %s: %v, skipping\n", entry.Path, err)
				continue
			}

			normalizedPath := core.NormalizePath(entry.Path)
			manifestEntry.StaticRoutes[normalizedPath] = "/pages/routes" + entry.Path + "/index.html"
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

// renderFullHTML generates a complete HTML page from rendered content
func renderFullHTML(page core.RenderedPage, entryName string, props map[string]any) string {
	// Build props JSON
	propsJSON := "{}"
	if len(props) > 0 {
		data, _ := json.Marshal(props)
		propsJSON = string(data)
	}

	// Generate HTML shell with rendered content
	html := fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />%s
    <link rel="stylesheet" href="/dist/%s.css" />
  </head>
  <body>
    <div id="app">%s</div>
    <script id="__BIFROST_PROPS__" type="application/json">%s</script>
    <script src="/dist/%s.js" type="module" defer></script>
  </body>
</html>`,
		page.Head,
		entryName,
		page.Body,
		propsJSON,
		entryName,
	)

	return html
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
