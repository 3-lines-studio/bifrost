package page

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/3-lines-studio/bifrost/internal/assets"
	"github.com/3-lines-studio/bifrost/internal/runtime"
	"github.com/3-lines-studio/bifrost/internal/types"
)

type Renderer interface {
	Render(componentPath string, props map[string]any) (types.RenderedPage, error)
	Build(entrypoints []string, outdir string) error
}

type Handler struct {
	renderer   Renderer
	config     types.PageConfig
	entryDir   string
	outdir     string
	entryPath  string
	entryName  string
	scriptSrc  string
	cssHref    string
	chunks     []string
	manifest   *assets.Manifest
	isDev      bool
	needsSetup bool
	setupErr   error
	setupOnce  sync.Once
	staticPath string
	ssrPath    string
	assetsFS   embed.FS
}

func NewHandler(renderer Renderer, config types.PageConfig, assetsFS embed.FS, isDev bool, manifest *assets.Manifest) http.Handler {
	paths := calculatePaths(config.ComponentPath)
	scriptSrc, cssHref, chunks, _, ssrPath := assets.GetAssets(manifest, paths.entryName)

	mode := config.Mode
	isClientOnly := mode == types.ModeClientOnly
	isStaticPrerender := mode == types.ModeStaticPrerender
	isStatic := isClientOnly || isStaticPrerender

	var staticPath string
	if isStatic && !isDev {
		staticPath = filepath.Join(".bifrost", "pages", paths.entryName, "index.html")
	}

	needsSetup := (manifest == nil || isDev) && !isStatic
	if isStatic && isDev && renderer != nil {
		needsSetup = true
	}

	return &Handler{
		renderer:   renderer,
		config:     config,
		entryDir:   paths.entryDir,
		outdir:     paths.outdir,
		entryPath:  paths.entryPath,
		entryName:  paths.entryName,
		scriptSrc:  scriptSrc,
		cssHref:    cssHref,
		chunks:     chunks,
		manifest:   manifest,
		isDev:      isDev,
		needsSetup: needsSetup,
		staticPath: staticPath,
		ssrPath:    ssrPath,
		assetsFS:   assetsFS,
	}
}

type pagePaths struct {
	entryDir     string
	outdir       string
	entryName    string
	entryPath    string
	manifestPath string
}

func calculatePaths(componentPath string) pagePaths {
	entryDir := ".bifrost"
	outdir := filepath.Join(entryDir, "dist")
	entryName := assets.EntryNameForPath(componentPath)
	entryPath := filepath.Join(entryDir, entryName+".tsx")
	manifestPath := filepath.Join(entryDir, "manifest.json")

	return pagePaths{
		entryDir:     entryDir,
		outdir:       outdir,
		entryName:    entryName,
		entryPath:    entryPath,
		manifestPath: manifestPath,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Production: Serve pre-built HTML for static modes
	if !h.isDev {
		switch h.config.Mode {
		case types.ModeClientOnly:
			if h.staticPath != "" {
				h.serveStaticFile(w, req)
				return
			}
		case types.ModeStaticPrerender:
			// Check for dynamic static routes
			if h.manifest != nil {
				entry := h.manifest.Entries[h.entryName]
				if entry.StaticRoutes != nil {
					// Normalize request path
					requestPath := normalizePath(req.URL.Path)
					// Look up in route map
					if htmlPath, ok := entry.StaticRoutes[requestPath]; ok {
						h.serveRouteFile(w, req, htmlPath)
						return
					}
					// Path not found in static routes - return 404
					http.NotFound(w, req)
					return
				}
			}
			// Fallback to simple static file serving
			if h.staticPath != "" {
				h.serveStaticFile(w, req)
				return
			}
		}
	}

	// Setup for dev mode (build bundles if needed)
	if h.needsSetup {
		if !h.handleSetup(w) {
			return
		}
	}

	// Dev mode: Handle StaticPrerender with StaticDataLoader
	if h.isDev && h.config.Mode == types.ModeStaticPrerender && h.config.StaticDataLoader != nil {
		requestPath := normalizePath(req.URL.Path)
		props, found, err := h.loadStaticDataForPath(requestPath)
		if err != nil {
			h.serveError(w, err)
			return
		}
		if !found {
			http.NotFound(w, req)
			return
		}
		// Render with the matched props
		renderPath := h.config.ComponentPath
		page, err := h.renderer.Render(renderPath, props)
		if err != nil {
			h.serveError(w, err)
			return
		}
		h.renderPage(w, props, page)
		return
	}

	loader := h.config.PropsLoader
	if loader == nil {
		loader = func(*http.Request) (map[string]any, error) {
			return map[string]any{}, nil
		}
	}

	props, err := loader(req)
	if err != nil {
		h.handlePropsError(w, req, err)
		return
	}

	renderPath, err := h.getRenderPath()
	if err != nil {
		h.serveError(w, err)
		return
	}

	page, err := h.renderer.Render(renderPath, props)
	if err != nil {
		h.serveError(w, err)
		return
	}

	h.renderPage(w, props, page)
}

func (h *Handler) serveStaticFile(w http.ResponseWriter, req *http.Request) {
	if h.assetsFS != (embed.FS{}) {
		data, err := h.assetsFS.ReadFile(h.staticPath)
		if err != nil {
			h.serveError(w, fmt.Errorf("failed to read static file: %w", err))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
		return
	}

	http.ServeFile(w, req, h.staticPath)
}

func (h *Handler) handleSetup(w http.ResponseWriter) bool {
	h.setupOnce.Do(func() {
		if h.config.Mode == types.ModeClientOnly {
			h.setupErr = h.setupStaticPage()
		} else {
			h.setupErr = h.setupSSRPage()
		}
	})
	return h.checkSetupError(w)
}

func (h *Handler) setupStaticPage() error {
	if h.renderer == nil {
		return nil
	}

	componentImport, err := assets.ComponentImportPath(h.entryPath, h.config.ComponentPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(h.entryDir, 0o755); err != nil {
		return err
	}

	if err := os.MkdirAll(h.outdir, 0o755); err != nil {
		return err
	}

	if err := WriteStaticClientEntry(h.entryPath, componentImport); err != nil {
		return err
	}

	if err := h.renderer.Build([]string{h.entryPath}, h.outdir); err != nil {
		return err
	}

	return nil
}

func (h *Handler) setupSSRPage() error {
	if h.renderer == nil {
		return nil
	}

	if h.config.ComponentPath == "" {
		return fmt.Errorf("missing component path")
	}

	componentImport, err := assets.ComponentImportPath(h.entryPath, h.config.ComponentPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(h.entryDir, 0o755); err != nil {
		return err
	}

	if err := os.MkdirAll(h.outdir, 0o755); err != nil {
		return err
	}

	if err := WriteClientEntry(h.entryPath, componentImport); err != nil {
		return err
	}

	if err := h.renderer.Build([]string{h.entryPath}, h.outdir); err != nil {
		return err
	}

	return nil
}

func (h *Handler) checkSetupError(w http.ResponseWriter) bool {
	if h.setupErr != nil {
		h.serveError(w, h.setupErr)
		return false
	}
	return true
}

func (h *Handler) handlePropsError(w http.ResponseWriter, req *http.Request, err error) {
	redirectErr, isRedirect := err.(types.RedirectError)
	if !isRedirect {
		h.serveError(w, err)
		return
	}

	status := redirectErr.RedirectStatusCode()
	if status == 0 {
		status = http.StatusFound
	}
	http.Redirect(w, req, redirectErr.RedirectURL(), status)
}

func (h *Handler) getRenderPath() (string, error) {
	// Dev mode: allow source TSX rendering
	if h.isDev {
		return h.config.ComponentPath, nil
	}

	// Production mode: require SSR bundle from manifest
	if h.ssrPath == "" {
		return "", fmt.Errorf("SSR bundle not found in manifest for %s; ensure you ran 'bifrost-build' and embedded the .bifrost directory", h.config.ComponentPath)
	}

	if h.assetsFS != (embed.FS{}) {
		resolver := assets.NewResolver(h.assetsFS, h.manifest, h.isDev)
		bundlePath := resolver.GetSSRBundlePath(h.ssrPath)
		if bundlePath == "" {
			return "", fmt.Errorf("failed to extract SSR bundle for %s from embedded assets", h.config.ComponentPath)
		}
		return bundlePath, nil
	}

	return filepath.Join(".bifrost", h.ssrPath), nil
}

func (h *Handler) renderPage(w http.ResponseWriter, props map[string]any, page types.RenderedPage) {
	fullHTML, err := RenderHTMLShell(page.Body, props, h.scriptSrc, page.Head, h.cssHref, h.chunks)
	if err != nil {
		h.serveError(w, err)
		return
	}

	serveHTML(w, fullHTML)
}

func serveHTML(w http.ResponseWriter, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}

type errorData struct {
	Message    string
	StackTrace string
	Errors     []errorDetail
	IsDev      bool
}

type errorDetail struct {
	Message   string
	File      string
	Line      int
	Column    int
	LineText  string
	Specifier string
	Referrer  string
}

func (h *Handler) serveError(w http.ResponseWriter, err error) {
	data := errorData{
		Message: "Internal Server Error",
		IsDev:   h.isDev,
	}

	if bifrostErr, ok := err.(*runtime.BifrostError); ok {
		data.Message = bifrostErr.Message
		data.StackTrace = bifrostErr.Stack
		data.Errors = make([]errorDetail, len(bifrostErr.Errors))
		for i, e := range bifrostErr.Errors {
			data.Errors[i] = errorDetail{
				Message:   e.Message,
				File:      e.File,
				Line:      e.Line,
				Column:    e.Column,
				LineText:  e.LineText,
				Specifier: e.Specifier,
				Referrer:  e.Referrer,
			}
		}
	} else if err != nil {
		data.Message = err.Error()
	}

	var buf bytes.Buffer
	if err := ErrorTemplate.Execute(&buf, data); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("<!doctype html><html><body><pre>" + html.EscapeString(data.Message) + "</pre></body></html>"))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	w.Write(buf.Bytes())
}

// normalizePath normalizes a URL path for route matching
func normalizePath(path string) string {
	// Ensure leading slash
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// Remove trailing slash (except for root)
	if path != "/" && strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}

// loadStaticDataForPath calls the StaticDataLoader and finds matching props for the request path
func (h *Handler) loadStaticDataForPath(requestPath string) (map[string]any, bool, error) {
	if h.config.StaticDataLoader == nil {
		return nil, false, nil
	}

	entries, err := h.config.StaticDataLoader(context.Background())
	if err != nil {
		return nil, false, err
	}

	for _, entry := range entries {
		if normalizePath(entry.Path) == requestPath {
			return entry.Props, true, nil
		}
	}

	return nil, false, nil
}

// serveRouteFile serves a specific route file from embedded assets
func (h *Handler) serveRouteFile(w http.ResponseWriter, req *http.Request, htmlPath string) {
	if h.assetsFS != (embed.FS{}) {
		// htmlPath is like "/pages/routes/blog/hello/index.html"
		// We need ".bifrost/pages/routes/blog/hello/index.html" for embed.FS
		embedPath := ".bifrost" + htmlPath
		embedPath = strings.TrimPrefix(embedPath, "/")
		embedPath = filepath.ToSlash(embedPath)
		data, err := h.assetsFS.ReadFile(embedPath)
		if err != nil {
			h.serveError(w, fmt.Errorf("failed to read route file %s: %w", embedPath, err))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
		return
	}

	// Fallback to filesystem
	fullPath := filepath.Join(".bifrost", htmlPath)
	http.ServeFile(w, req, fullPath)
}
