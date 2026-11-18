package bifrost

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/3-lines-studio/bifrost/internal/dochtml"
)

// Register installs Bifrost pages and assets into mux.
func (a *App) Register(mux *http.ServeMux) (err error) {
	if mux == nil {
		return errors.New("bifrost: nil ServeMux")
	}
	if a == nil || a.runtime == nil {
		return errors.New("bifrost: app has no compiled runtime")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("bifrost: register handlers: %v", recovered)
		}
	}()
	patterns := make([]string, 0, len(a.runtime.handlers))
	for pattern := range a.runtime.handlers {
		patterns = append(patterns, pattern)
	}
	slices.Sort(patterns)
	for _, pattern := range patterns {
		mux.Handle("GET "+pattern, a.runtime.handlers[pattern])
	}
	if a.site != nil {
		mux.HandleFunc("GET /sitemap.xml", a.serveSitemap)
		mux.HandleFunc("GET /robots.txt", a.serveRobots)
	}
	mux.HandleFunc("GET "+dochtml.AssetPrefix+"ready", func(w http.ResponseWriter, request *http.Request) {
		if err := a.Ready(request.Context()); err != nil {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if a.runtime.manifest != nil {
		mux.HandleFunc("GET "+dochtml.AssetPrefix+"build-id", func(w http.ResponseWriter, request *http.Request) {
			id := a.runtime.manifest.manifest.BuildID
			if request.URL.Query().Get("wait") == id && a.runtime.devProxy != nil {
				select {
				case <-request.Context().Done():
					return
				case <-time.After(30 * time.Second):
				}
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write([]byte(id))
		})
		if a.runtime.devProxy != nil {
			mux.Handle("GET "+strings.TrimSuffix(dochtml.DevPrefix, "/")+"/{path...}", a.runtime.devProxy)
		}
	}
	if a.runtime.assets != nil {
		mux.Handle("GET "+dochtml.AssetPrefix+"{path...}", &assetHandler{assets: a.runtime.assets, files: a.runtime.files, headers: a.hooks.assetHeaders})
		publicURLs := make([]string, 0, len(a.runtime.public))
		for publicURL := range a.runtime.public {
			publicURLs = append(publicURLs, publicURL)
		}
		slices.Sort(publicURLs)
		for _, publicURL := range publicURLs {
			mux.Handle("GET "+publicURL, &publicAssetHandler{assets: a.runtime.assets, file: a.runtime.public[publicURL], headers: a.hooks.assetHeaders})
		}
	}
	return nil
}

// Handler returns a standalone Bifrost handler. A setup failure produces a
// deterministic 503 response rather than a partially configured route table.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	if err := a.Register(mux); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		})
	}
	return a.ResolveMarkdown(mux)
}

// Ready checks whether every renderer process can accept work. Applications
// with only Static or Client routes are ready when their runtime is compiled.
func (a *App) Ready(ctx context.Context) error {
	if a == nil || a.runtime == nil {
		return errors.New("bifrost: app has no compiled runtime")
	}
	if a.runtime.renderer == nil {
		return nil
	}
	ready, ok := a.runtime.renderer.(interface{ Ready(context.Context) error })
	if !ok {
		return errors.New("bifrost: renderer does not report readiness")
	}
	return ready.Ready(ctx)
}

// Close drains renderer work until ctx ends, then stops child processes.
func (a *App) Close(ctx context.Context) error {
	if a == nil || a.runtime == nil || a.runtime.renderer == nil {
		return nil
	}
	return a.runtime.renderer.Close(ctx)
}
