package bifrost

import (
	"embed"
	"net/http"

	"github.com/3-lines-studio/bifrost/internal/bifrost"
)

type Renderer = bifrost.Renderer

type Page = bifrost.Page

type Option func(*bifrost.Renderer)

func WithAssetsFS(fs embed.FS) Option {
	return func(r *bifrost.Renderer) {
		r.SetAssetsFS(fs)
	}
}

func WithTiming() Option {
	return func(r *bifrost.Renderer) {
		r.SetTimingEnabled(true)
	}
}

func New(opts ...Option) (*bifrost.Renderer, error) {
	r, err := bifrost.NewRenderer()
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		opt(r)
	}
	return r, nil
}

func RegisterAssetRoutes(r bifrost.Router, renderer *bifrost.Renderer, appRouter http.Handler) {
	if bifrost.IsDev() {
		assets := bifrost.AssetHandler()
		r.Handle("/dist/*", assets)
		r.Handle("/__bifrost_reload", bifrost.DevHandler())
		r.Handle("/*", bifrost.PublicHandler(renderer.AssetsFS, appRouter))
	} else if renderer.AssetsFS != (embed.FS{}) {
		assets := bifrost.EmbeddedAssetHandler(renderer.AssetsFS)
		r.Handle("/dist/*", assets)
		r.Handle("/*", bifrost.PublicHandler(renderer.AssetsFS, appRouter))
	}
}
