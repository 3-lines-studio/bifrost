package bifrost

import (
	"embed"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/3-lines-studio/bifrost/internal/assets"
	"github.com/3-lines-studio/bifrost/internal/page"
	"github.com/3-lines-studio/bifrost/internal/runtime"
	"github.com/3-lines-studio/bifrost/internal/types"
)

const exportMarkerPath = ".bifrost/.export-mode"

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

type RedirectError = types.RedirectError

type StaticPathData = types.StaticPathData

type PageOption = types.PageOption

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
	manifest    *assets.Manifest
	pageConfigs map[string]*types.PageConfig
}

type router interface {
	http.Handler
	Handle(pattern string, handler http.Handler)
}

func New(assetsFS embed.FS, routes ...Route) *App {
	mode := runtime.GetMode()

	app := &App{
		assetsFS:    assetsFS,
		isDev:       mode == runtime.ModeDev,
		routes:      routes,
		pageConfigs: make(map[string]*types.PageConfig),
	}

	if isExportMode() {
		for _, route := range routes {
			config := types.PageConfig{
				ComponentPath: route.ComponentPath,
				Mode:          types.ModeSSR,
			}
			for _, opt := range route.Options {
				opt(&config)
			}
			app.pageConfigs[route.ComponentPath] = &config
		}
		return app
	}

	r, err := newRenderer(assetsFS, mode)
	if err != nil {
		panic(fmt.Sprintf("failed to create bifrost renderer: %v", err))
	}
	app.renderer = r
	app.manifest = r.manifest

	for _, route := range routes {
		config := types.PageConfig{
			ComponentPath: route.ComponentPath,
			Mode:          types.ModeSSR,
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
		config := types.PageConfig{
			ComponentPath: route.ComponentPath,
			Mode:          types.ModeSSR,
		}
		for _, opt := range route.Options {
			opt(&config)
		}

		handler := page.NewHandler(a.renderer.client, config, a.assetsFS, a.isDev, a.manifest)
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

func Page(pattern string, componentPath string, opts ...PageOption) Route {
	return Route{
		Pattern:       pattern,
		ComponentPath: componentPath,
		Options:       opts,
	}
}

func WithLoader(loader types.PropsLoader) PageOption {
	return types.WithLoader(loader)
}

func WithClient() PageOption {
	return types.WithClient()
}

func WithStatic() PageOption {
	return types.WithStatic()
}

func WithStaticData(loader types.StaticDataLoader) PageOption {
	return types.WithStaticData(loader)
}

type renderer struct {
	client   *runtime.Client
	assetsFS embed.FS
	isDev    bool
	manifest *assets.Manifest
}

func newRenderer(assetsFS embed.FS, mode runtime.Mode) (*renderer, error) {
	r := &renderer{
		isDev:    mode == runtime.ModeDev,
		assetsFS: assetsFS,
	}

	if mode == runtime.ModeProd {
		if assetsFS == (embed.FS{}) {
			return nil, runtime.ErrAssetsFSRequiredInProd
		}

		man, err := assets.LoadManifestFromEmbed(assetsFS, ".bifrost/manifest.json")
		if err != nil {
			return nil, fmt.Errorf("%w: %v", runtime.ErrManifestMissingInAssetsFS, err)
		}
		r.manifest = man

		needsRuntime := assets.HasSSREntries(man)

		if needsRuntime {
			if !runtime.HasEmbeddedRuntime(assetsFS) {
				return nil, runtime.ErrEmbeddedRuntimeNotFound
			}

			executablePath, cleanup, err := runtime.ExtractEmbeddedRuntime(assetsFS)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", runtime.ErrEmbeddedRuntimeExtraction, err)
			}

			client, err := runtime.NewClientFromExecutable(executablePath, cleanup)
			if err != nil {
				cleanup()
				return nil, fmt.Errorf("%w: %v", runtime.ErrEmbeddedRuntimeStart, err)
			}
			r.client = client
		}
	} else {
		client, err := runtime.NewClient()
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

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		path := req.URL.Path

		if len(path) >= 6 && path[:6] == "/dist/" {
			if isDev {
				assets.AssetHandler().ServeHTTP(w, req)
			} else if app.assetsFS != (embed.FS{}) {
				assets.EmbeddedAssetHandler(app.assetsFS).ServeHTTP(w, req)
			}
			return
		}

		router.ServeHTTP(w, req)
	})
}
