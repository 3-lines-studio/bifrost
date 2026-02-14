package bifrost

import (
	"embed"
	"fmt"
	"net/http"
	"os"

	"github.com/3-lines-studio/bifrost/internal/assets"
	"github.com/3-lines-studio/bifrost/internal/page"
	"github.com/3-lines-studio/bifrost/internal/runtime"
	"github.com/3-lines-studio/bifrost/internal/types"
)

type RedirectError = types.RedirectError

type PageOption = types.PageOption

type PageMode = types.PageMode

const (
	ModeSSR             = types.ModeSSR
	ModeClientOnly      = types.ModeClientOnly
	ModeStaticPrerender = types.ModeStaticPrerender
)

func WithPropsLoader(loader types.PropsLoader) PageOption {
	return types.WithPropsLoader(loader)
}

func WithClientOnly() PageOption {
	return types.WithClientOnly()
}

func WithStaticPrerender() PageOption {
	return types.WithStaticPrerender()
}

func WithStaticDataLoader(loader types.StaticDataLoader) PageOption {
	return types.WithStaticDataLoader(loader)
}

// Export types for use with WithStaticDataLoader
type StaticPathData = types.StaticPathData
type StaticDataLoader = types.StaticDataLoader

type Renderer struct {
	client      *runtime.Client
	assetsFS    embed.FS
	isDev       bool
	manifest    *assets.Manifest
	pageConfigs map[string]*types.PageConfig // ComponentPath -> Config
}

type Option func(*Renderer)

func WithAssetsFS(fs embed.FS) Option {
	return func(r *Renderer) {
		r.assetsFS = fs
	}
}

func New(opts ...Option) (*Renderer, error) {
	mode := runtime.GetMode()

	// Check for export mode (build-time static data export)
	if os.Getenv("BIFROST_EXPORT_STATIC") == "1" {
		return &Renderer{
			isDev:       false,
			pageConfigs: make(map[string]*types.PageConfig),
		}, nil
	}

	r := &Renderer{
		isDev:       mode == runtime.ModeDev,
		pageConfigs: make(map[string]*types.PageConfig),
	}

	for _, opt := range opts {
		opt(r)
	}

	// Strict production validation
	if mode == runtime.ModeProd {
		if r.assetsFS == (embed.FS{}) {
			return nil, runtime.ErrAssetsFSRequiredInProd
		}

		man, err := assets.LoadManifestFromEmbed(r.assetsFS, ".bifrost/manifest.json")
		if err != nil {
			return nil, fmt.Errorf("%w: %v", runtime.ErrManifestMissingInAssetsFS, err)
		}
		r.manifest = man

		// In production, use embedded runtime helper
		if !runtime.HasEmbeddedRuntime(r.assetsFS) {
			return nil, runtime.ErrEmbeddedRuntimeNotFound
		}

		executablePath, cleanup, err := runtime.ExtractEmbeddedRuntime(r.assetsFS)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", runtime.ErrEmbeddedRuntimeExtraction, err)
		}

		client, err := runtime.NewClientFromExecutable(executablePath, cleanup)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("%w: %v", runtime.ErrEmbeddedRuntimeStart, err)
		}
		r.client = client
	} else {
		// Development mode: use system Bun
		client, err := runtime.NewClient()
		if err != nil {
			return nil, err
		}
		r.client = client
	}

	return r, nil
}

func (r *Renderer) Stop() error {
	if r.client != nil {
		return r.client.Stop()
	}
	return nil
}

func (r *Renderer) Render(componentPath string, props map[string]any) (types.RenderedPage, error) {
	return r.client.Render(componentPath, props)
}

func (r *Renderer) NewPage(componentPath string, opts ...PageOption) http.Handler {
	config := types.PageConfig{
		ComponentPath: componentPath,
		Mode:          types.ModeSSR,
	}

	for _, opt := range opts {
		opt(&config)
	}

	// Store config for export mode
	r.pageConfigs[componentPath] = &config

	return page.NewHandler(r.client, config, r.assetsFS, r.isDev, r.manifest)
}

type Router interface {
	Handle(pattern string, handler http.Handler)
}

func RegisterAssetRoutes(r Router, renderer *Renderer, appRouter http.Handler) {
	isDev := runtime.GetMode() == runtime.ModeDev

	if isDev {
		assetsHandler := assets.AssetHandler()
		r.Handle("/dist/*", assetsHandler)
		if renderer != nil {
			r.Handle("/*", assets.PublicHandler(renderer.assetsFS, appRouter, isDev))
		} else {
			r.Handle("/*", appRouter)
		}
	} else if renderer != nil && renderer.assetsFS != (embed.FS{}) {
		assetsHandler := assets.EmbeddedAssetHandler(renderer.assetsFS)
		r.Handle("/dist/*", assetsHandler)
		r.Handle("/*", assets.PublicHandler(renderer.assetsFS, appRouter, isDev))
	} else {
		r.Handle("/*", appRouter)
	}
}
