package bifrost

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	adaptersfs "github.com/3-lines-studio/bifrost/internal/adapters/fs"
	adaptershttp "github.com/3-lines-studio/bifrost/internal/adapters/http"
	"github.com/3-lines-studio/bifrost/internal/adapters/process"
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

var (
	exportModeOnce sync.Once
	exportMode     bool
)

func isExportMode() bool {
	exportModeOnce.Do(func() {
		_, err := os.Stat(exportMarkerPath)
		exportMode = err == nil
	})
	return exportMode
}

type RedirectError = core.RedirectError

type StaticPathData = core.StaticPathData

type PageOption = core.PageOption

type Route struct {
	Pattern       string
	ComponentPath string
	Options       []PageOption
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

	if isExportMode() {
		for _, route := range routes {
			config := core.PageConfig{
				ComponentPath: route.ComponentPath,
				Mode:          core.ModeSSR,
			}
			for _, opt := range route.Options {
				opt(&config)
			}
			app.pageConfigs[route.ComponentPath] = &config
		}
		return app
	}

	// Handle export mode
	if mode == core.ModeExport {
		// Initialize renderer for export (reads manifest from disk)
		r, err := newRenderer(assetsFS, core.ModeExport)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Export failed: %v\n", err)
			os.Exit(1)
		}
		app.renderer = r
		app.manifest = r.manifest

		// Run export
		outputDir := os.Getenv("BIFROST_EXPORT_DIR")
		if outputDir == "" {
			outputDir = ".bifrost"
		}

		if err := app.ExportStaticPages(outputDir); err != nil {
			fmt.Fprintf(os.Stderr, "Export failed: %v\n", err)
			os.Exit(1)
		}

		app.Stop()
		os.Exit(0)
	}

	r, err := newRenderer(assetsFS, mode)
	if err != nil {
		panic(fmt.Sprintf("failed to create bifrost renderer: %v", err))
	}
	app.renderer = r
	app.manifest = r.manifest

	for _, route := range routes {
		config := core.PageConfig{
			ComponentPath: route.ComponentPath,
			Mode:          core.ModeSSR,
		}
		for _, opt := range route.Options {
			opt(&config)
		}
		app.pageConfigs[route.ComponentPath] = &config
	}

	return app
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
		config := core.PageConfig{
			ComponentPath: route.ComponentPath,
			Mode:          core.ModeSSR,
		}
		for _, opt := range route.Options {
			opt(&config)
		}

		fsAdapter := adaptersfs.NewEmbedFileSystem(a.assetsFS)
		pageService := usecase.NewPageService(a.renderer.client, fsAdapter)

		// Get static path from manifest based on page mode
		staticPath := ""
		if a.manifest != nil {
			entryName := core.EntryNameForPath(config.ComponentPath)
			if entry, ok := a.manifest.Entries[entryName]; ok {
				switch config.Mode {
				case core.ModeClientOnly:
					// ClientOnly pages use pre-built HTML shell
					staticPath = entry.HTML
				default:
					// SSR and StaticPrerender pages use SSR bundle
					staticPath = entry.SSR
					// In production, prepend the temp directory to get absolute path
					if !a.isDev && a.renderer != nil && a.renderer.ssrTempDir != "" && staticPath != "" {
						staticPath = filepath.Join(a.renderer.ssrTempDir, staticPath)
					}
				}
			}
		}

		handler := adaptershttp.NewPageHandler(pageService, config, a.manifest, a.assetsFS, a.isDev, staticPath)
		api.Handle(route.Pattern, handler)
	}

	return createAssetHandler(api, a)
}

func (a *App) Handler() http.Handler {
	return a.Wrap(http.NewServeMux())
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

	// Process each route
	for _, route := range a.routes {
		config := core.PageConfig{
			ComponentPath: route.ComponentPath,
			Mode:          core.ModeSSR,
		}
		for _, opt := range route.Options {
			opt(&config)
		}

		// Only process StaticPrerender pages
		if config.Mode != core.ModeStaticPrerender {
			continue
		}

		if config.StaticDataLoader == nil {
			fmt.Printf("Warning: No StaticDataLoader for %s, skipping\n", route.Pattern)
			continue
		}

		entryName := core.EntryNameForPath(config.ComponentPath)

		// Get SSR bundle path from manifest
		ssrBundlePath := ""
		if a.manifest != nil {
			if entry, ok := a.manifest.Entries[entryName]; ok {
				ssrBundlePath = entry.SSR
			}
		}

		if ssrBundlePath == "" {
			fmt.Printf("Warning: No SSR bundle for %s, skipping\n", route.Pattern)
			continue
		}

		// Build full path to extracted SSR bundle
		if a.renderer != nil && a.renderer.ssrTempDir != "" {
			ssrBundlePath = filepath.Join(a.renderer.ssrTempDir, ssrBundlePath)
		}

		// Get all static paths
		entries, err := config.StaticDataLoader(context.Background())
		if err != nil {
			fmt.Printf("Warning: Failed to load static data for %s: %v, skipping\n", route.Pattern, err)
			continue
		}

		manifestEntry := core.ManifestEntry{
			Script:       a.manifest.Entries[entryName].Script,
			CSS:          a.manifest.Entries[entryName].CSS,
			Mode:         "static",
			StaticRoutes: make(map[string]string),
		}

		// Render each path
		for _, entry := range entries {
			fmt.Printf("Exporting %s...\n", entry.Path)

			// Render page
			page, err := a.renderer.client.Render(ssrBundlePath, entry.Props)
			if err != nil {
				fmt.Printf("Warning: Failed to render %s: %v, skipping\n", entry.Path, err)
				continue
			}

			// Generate full HTML
			html := renderFullHTML(page, entryName, entry.Props)

			// Write to file
			htmlPath := filepath.Join(pagesDir, entry.Path, "index.html")
			if err := os.MkdirAll(filepath.Dir(htmlPath), 0755); err != nil {
				fmt.Printf("Warning: Failed to create directory for %s: %v, skipping\n", entry.Path, err)
				continue
			}

			if err := os.WriteFile(htmlPath, []byte(html), 0644); err != nil {
				fmt.Printf("Warning: Failed to write %s: %v, skipping\n", entry.Path, err)
				continue
			}

			// Update manifest
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

type renderer struct {
	client     *process.BunRuntime
	assetsFS   embed.FS
	isDev      bool
	manifest   *core.Manifest
	ssrTempDir string
	ssrCleanup func()
}

func copySSRBundlesFromDisk(exportDir string, manifest *core.Manifest) (string, func(), error) {
	tempDir, err := os.MkdirTemp("", "bifrost-ssr-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create SSR temp dir: %w", err)
	}

	for entryName, entry := range manifest.Entries {
		if entry.SSR == "" {
			continue
		}

		srcPath := filepath.Join(exportDir, entry.SSR)

		data, err := os.ReadFile(srcPath)
		if err != nil {
			os.RemoveAll(tempDir)
			return "", nil, fmt.Errorf("failed to read SSR bundle %s: %w", srcPath, err)
		}

		destPath := filepath.Join(tempDir, entry.SSR)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			os.RemoveAll(tempDir)
			return "", nil, fmt.Errorf("failed to create SSR dest dir: %w", err)
		}

		if err := os.WriteFile(destPath, data, 0644); err != nil {
			os.RemoveAll(tempDir)
			return "", nil, fmt.Errorf("failed to write SSR bundle %s: %w", entryName, err)
		}
	}

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	return tempDir, cleanup, nil
}

func newRenderer(assetsFS embed.FS, mode core.Mode) (*renderer, error) {
	r := &renderer{
		isDev:    mode == core.ModeDev,
		assetsFS: assetsFS,
	}

	if mode == core.ModeExport {
		exportDir := os.Getenv("BIFROST_EXPORT_DIR")
		if exportDir == "" {
			exportDir = ".bifrost"
		}

		manifestPath := filepath.Join(exportDir, "manifest.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("manifest.json not found at %s: %w", manifestPath, err)
		}

		man, err := core.ParseManifest(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse manifest: %w", err)
		}
		r.manifest = man

		needsRuntime := core.HasSSREntries(man)

		if needsRuntime {
			ssrTempDir, ssrCleanup, err := copySSRBundlesFromDisk(exportDir, man)
			if err != nil {
				return nil, fmt.Errorf("failed to copy SSR bundles: %w", err)
			}
			r.ssrTempDir = ssrTempDir
			r.ssrCleanup = ssrCleanup

			client, err := process.NewBunRuntime(core.ModeProd)
			if err != nil {
				ssrCleanup()
				return nil, fmt.Errorf("failed to start bun runtime: %w", err)
			}
			r.client = client
		}

		return r, nil
	}

	if mode == core.ModeProd {
		if assetsFS == (embed.FS{}) {
			return nil, fmt.Errorf("embed.FS is required in production mode")
		}

		data, err := assetsFS.ReadFile(".bifrost/manifest.json")
		if err != nil {
			return nil, fmt.Errorf("manifest.json not found in embedded assets: %w", err)
		}

		man, err := core.ParseManifest(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse manifest: %w", err)
		}
		r.manifest = man

		needsRuntime := core.HasSSREntries(man)

		if needsRuntime {
			if !process.HasEmbeddedRuntime(assetsFS) {
				return nil, fmt.Errorf("embedded runtime not found: run 'bifrost-build' to generate production assets")
			}

			ssrTempDir, ssrCleanup, err := process.ExtractSSRBundles(assetsFS, man)
			if err != nil {
				return nil, fmt.Errorf("failed to extract SSR bundles: %w", err)
			}
			r.ssrTempDir = ssrTempDir
			r.ssrCleanup = ssrCleanup

			executablePath, cleanup, err := process.ExtractEmbeddedRuntime(assetsFS)
			if err != nil {
				ssrCleanup()
				return nil, fmt.Errorf("failed to extract embedded runtime: %w", err)
			}

			combinedCleanup := func() {
				cleanup()
				ssrCleanup()
			}

			client, err := process.NewBunRuntimeFromExecutable(executablePath, combinedCleanup)
			if err != nil {
				cleanup()
				ssrCleanup()
				return nil, fmt.Errorf("failed to start embedded runtime: %w", err)
			}
			r.client = client
		}
	} else {
		client, err := process.NewBunRuntime(mode)
		if err != nil {
			return nil, err
		}
		r.client = client
	}

	return r, nil
}

func (r *renderer) stop() error {
	if r.client != nil {
		return r.client.Stop()
	}
	return nil
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
