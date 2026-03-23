package http

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"path"
	"path/filepath"

	"github.com/3-lines-studio/bifrost/internal/core"
	"github.com/3-lines-studio/bifrost/internal/usecase"
)

type PageHandler struct {
	service         *usecase.PageService
	config          core.PageConfig
	manifest        *core.Manifest
	assetsFS        embed.FS
	isDev           bool
	entryName       string
	staticPath      string
	defaultHTMLLang string
}

func NewPageHandler(
	service *usecase.PageService,
	config core.PageConfig,
	manifest *core.Manifest,
	assetsFS embed.FS,
	isDev bool,
	staticPath string,
	defaultHTMLLang string,
) http.Handler {
	return &PageHandler{
		service:         service,
		config:          config,
		manifest:        manifest,
		assetsFS:        assetsFS,
		isDev:           isDev,
		entryName:       core.EntryNameForPath(config.ComponentPath),
		staticPath:      staticPath,
		defaultHTMLLang: defaultHTMLLang,
	}
}

var errNeedsSetup = errors.New("page needs setup but setup not implemented in adapter")

func (h *PageHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	input := usecase.ServePageInput{
		Config:          h.config,
		DefaultHTMLLang: h.defaultHTMLLang,
		IsDev:           h.isDev,
		Manifest:        h.manifest,
		EntryName:       h.entryName,
		StaticPath:      h.staticPath,
		RequestPath:     req.URL.Path,
		Request:         req,
	}

	output := h.service.ServePage(req.Context(), input)

	if output.Error != nil {
		h.serveError(w, req, output.Error)
		return
	}

	switch output.Action {
	case core.ActionServeStaticFile:
		h.serveBifrostHTMLFile(w, req, output.StaticPath, "static")

	case core.ActionServeRouteFile:
		h.serveBifrostHTMLFile(w, req, output.RoutePath, "route")

	case core.ActionNotFound:
		http.NotFound(w, req)

	case core.ActionNeedsSetup:
		h.serveError(w, req, errNeedsSetup)

	case core.ActionRenderSSR:
		if output.Stream != nil {
			if err := output.Stream(w); err != nil {
				h.serveError(w, req, err)
			}
			return
		}
		h.serveHTML(w, output.HTML)

	case core.ActionRenderClientOnlyShell,
		core.ActionRenderStaticPrerender:
		h.serveHTML(w, output.HTML)
	}
}

func (h *PageHandler) serveBifrostHTMLFile(w http.ResponseWriter, req *http.Request, logicalPath string, kind string) {
	rel, ok := cleanPath(logicalPath)
	if !ok {
		h.serveError(w, req, fmt.Errorf("invalid %s file path: %s", kind, logicalPath))
		return
	}
	if h.assetsFS != (embed.FS{}) {
		embedPath := path.Join(".bifrost", rel)
		data, err := h.assetsFS.ReadFile(embedPath)
		if err != nil {
			h.serveError(w, req, fmt.Errorf("failed to read %s file %s: %w", kind, embedPath, err))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
		return
	}

	safePath := filepath.Join(".bifrost", filepath.FromSlash(rel))
	if !isPathSafe(safePath, ".bifrost") {
		http.NotFound(w, req)
		return
	}
	http.ServeFile(w, req, safePath)
}

func (h *PageHandler) serveHTML(w http.ResponseWriter, htmlContent string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, htmlContent)
}

func (h *PageHandler) serveError(w http.ResponseWriter, req *http.Request, err error) {
	if redirectErr, ok := err.(core.RedirectError); ok {
		status := redirectErr.RedirectStatusCode()
		if status == 0 {
			status = http.StatusFound
		}
		http.Redirect(w, req, redirectErr.RedirectURL(), status)
		return
	}

	data := core.ErrorData{
		Message: err.Error(),
		IsDev:   h.isDev,
	}

	var buf bytes.Buffer
	if err := core.ErrorTemplate.Execute(&buf, data); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "<!doctype html><html><body><pre>"+html.EscapeString(data.Message)+"</pre></body></html>")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write(buf.Bytes())
}
