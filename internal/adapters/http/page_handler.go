package http

import (
	"bytes"
	"embed"
	"fmt"
	"html"
	"html/template"
	"net/http"
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
		h.serveError(w, output.Error)
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
		h.serveError(w, fmt.Errorf("page needs setup but setup not implemented in adapter"))

	case core.ActionRenderClientOnlyShell,
		core.ActionRenderStaticPrerender,
		core.ActionRenderSSR:
		h.serveHTML(w, output.HTML)
	}
}

func (h *PageHandler) serveStaticFile(w http.ResponseWriter, req *http.Request, path string) {
	if h.assetsFS != (embed.FS{}) {
		// Convert path to embedded FS format (e.g., "/pages/file.html" -> ".bifrost/pages/file.html")
		embedPath := ".bifrost" + path
		embedPath = strings.TrimPrefix(embedPath, "/")
		embedPath = filepath.ToSlash(embedPath)
		data, err := h.assetsFS.ReadFile(embedPath)
		if err != nil {
			h.serveError(w, fmt.Errorf("failed to read static file %s: %w", embedPath, err))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
		return
	}

	http.ServeFile(w, req, path)
}

func (h *PageHandler) serveRouteFile(w http.ResponseWriter, req *http.Request, htmlPath string) {
	if h.assetsFS != (embed.FS{}) {
		embedPath := ".bifrost" + htmlPath
		embedPath = strings.TrimPrefix(embedPath, "/")
		embedPath = filepath.ToSlash(embedPath)
		data, err := h.assetsFS.ReadFile(embedPath)
		if err != nil {
			h.serveError(w, fmt.Errorf("failed to read route file %s: %w", embedPath, err))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
		return
	}

	fullPath := filepath.Join(".bifrost", htmlPath)
	http.ServeFile(w, req, fullPath)
}

func (h *PageHandler) serveNotFound(w http.ResponseWriter, req *http.Request) {
	http.NotFound(w, req)
}

func (h *PageHandler) serveHTML(w http.ResponseWriter, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

func (h *PageHandler) serveError(w http.ResponseWriter, err error) {
	data := errorData{
		Message: err.Error(),
		IsDev:   h.isDev,
	}

	var buf bytes.Buffer
	if err := errorTemplate.Execute(&buf, data); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("<!doctype html><html><body><pre>" + html.EscapeString(data.Message) + "</pre></body></html>"))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write(buf.Bytes())
}

type errorData struct {
	Message string
	IsDev   bool
}

var errorTemplate = template.Must(template.New("error").Parse(`<!doctype html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Error</title>
    <style>
        body { font-family: system-ui, sans-serif; max-width: 800px; margin: 50px auto; padding: 0 20px; }
        h1 { color: #e74c3c; }
        pre { background: #f8f9fa; padding: 15px; border-radius: 5px; overflow-x: auto; }
    </style>
</head>
<body>
    <h1>Internal Server Error</h1>
    {{if .IsDev}}
    <pre>{{.Message}}</pre>
    {{else}}
    <p>An error occurred while processing your request.</p>
    {{end}}
</body>
</html>`))
