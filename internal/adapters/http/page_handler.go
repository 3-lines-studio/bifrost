package http

import (
	"bytes"
	"embed"
	"fmt"
	"html"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/core"
	"github.com/3-lines-studio/bifrost/internal/usecase"
)

type PageHandler struct {
	service    *usecase.PageService
	config     core.PageConfig
	manifest   *core.Manifest
	assetsFS   embed.FS
	isDev      bool
	entryName  string
	staticPath string
}

func NewPageHandler(
	service *usecase.PageService,
	config core.PageConfig,
	manifest *core.Manifest,
	assetsFS embed.FS,
	isDev bool,
	staticPath string,
) http.Handler {
	return &PageHandler{
		service:    service,
		config:     config,
		manifest:   manifest,
		assetsFS:   assetsFS,
		isDev:      isDev,
		entryName:  core.EntryNameForPath(config.ComponentPath),
		staticPath: staticPath,
	}
}

func (h *PageHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	input := usecase.ServePageInput{
		Config:      h.config,
		IsDev:       h.isDev,
		Manifest:    h.manifest,
		EntryName:   h.entryName,
		StaticPath:  h.staticPath,
		RequestPath: req.URL.Path,
		Request:     req,
	}

	output := h.service.ServePage(req.Context(), input)

	if output.Error != nil {
		h.serveError(w, req, output.Error)
		return
	}

	switch output.Action {
	case core.ActionServeStaticFile:
		h.serveStaticFile(w, req, output.StaticPath)

	case core.ActionServeRouteFile:
		h.serveRouteFile(w, req, output.RoutePath)

	case core.ActionNotFound:
		h.serveNotFound(w, req)

	case core.ActionNeedsSetup:
		h.serveError(w, req, fmt.Errorf("page needs setup but setup not implemented in adapter"))

	case core.ActionRenderClientOnlyShell,
		core.ActionRenderStaticPrerender,
		core.ActionRenderSSR:
		h.serveHTML(w, output.HTML)
	}
}

// safeEmbedPath builds a safe embedded FS path rooted under ".bifrost".
// Returns the cleaned embed path and true, or empty string and false if unsafe.
func safeEmbedPath(raw string) (string, bool) {
	if containsDotDot(strings.ReplaceAll(raw, "\\", "/")) {
		return "", false
	}
	cleaned := path.Clean("/" + raw)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" || cleaned == "." {
		return "", false
	}
	return path.Join(".bifrost", cleaned), true
}


func (h *PageHandler) serveStaticFile(w http.ResponseWriter, req *http.Request, p string) {
	if h.assetsFS != (embed.FS{}) {
		embedPath, ok := safeEmbedPath(p)
		if !ok {
			h.serveError(w, req, fmt.Errorf("invalid static file path: %s", p))
			return
		}
		data, err := h.assetsFS.ReadFile(embedPath)
		if err != nil {
			h.serveError(w, req, fmt.Errorf("failed to read static file %s: %w", embedPath, err))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
		return
	}

	safePath := filepath.Join(".bifrost", filepath.FromSlash(path.Clean("/"+p)))
	abs, err := filepath.Abs(safePath)
	if err != nil {
		http.NotFound(w, req)
		return
	}
	root, err := filepath.Abs(".bifrost")
	if err != nil {
		http.NotFound(w, req)
		return
	}
	if !strings.HasPrefix(abs, root+string(filepath.Separator)) && abs != root {
		http.NotFound(w, req)
		return
	}
	http.ServeFile(w, req, safePath)
}

func (h *PageHandler) serveRouteFile(w http.ResponseWriter, req *http.Request, htmlPath string) {
	if h.assetsFS != (embed.FS{}) {
		embedPath, ok := safeEmbedPath(htmlPath)
		if !ok {
			h.serveError(w, req, fmt.Errorf("invalid route file path: %s", htmlPath))
			return
		}
		data, err := h.assetsFS.ReadFile(embedPath)
		if err != nil {
			h.serveError(w, req, fmt.Errorf("failed to read route file %s: %w", embedPath, err))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
		return
	}

	safePath := filepath.Join(".bifrost", filepath.FromSlash(path.Clean("/"+htmlPath)))
	abs, err := filepath.Abs(safePath)
	if err != nil {
		http.NotFound(w, req)
		return
	}
	root, err := filepath.Abs(".bifrost")
	if err != nil {
		http.NotFound(w, req)
		return
	}
	if !strings.HasPrefix(abs, root+string(filepath.Separator)) && abs != root {
		http.NotFound(w, req)
		return
	}
	http.ServeFile(w, req, safePath)
}

func (h *PageHandler) serveNotFound(w http.ResponseWriter, req *http.Request) {
	http.NotFound(w, req)
}

func (h *PageHandler) serveHTML(w http.ResponseWriter, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
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
		_, _ = w.Write([]byte("<!doctype html><html><body><pre>" + html.EscapeString(data.Message) + "</pre></body></html>"))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write(buf.Bytes())
}
