package bifrost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/3-lines-studio/bifrost/internal/dochtml"
	"github.com/3-lines-studio/bifrost/internal/protocol"
)

type renderRequest struct {
	Pattern string
	Entry   string
	Props   json.RawMessage
}

type renderSink interface {
	Head([]byte) error
	Body([]byte) error
}

type pageRenderSink interface {
	renderSink
	finish() error
	committed() bool
}

type renderer interface {
	Render(context.Context, renderRequest, renderSink) error
	Close(context.Context) error
}

type runtimeState struct {
	assets         fs.FS
	manifest       *compiledManifest
	renderer       renderer
	handlers       map[string]http.Handler
	serverPatterns map[string]struct{}
	files          map[string]protocol.FileRef
	public         map[string]protocol.FileRef
	devProxy       http.Handler
}

func newDevelopmentProxy(port int) http.Handler {
	target := &url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(port)}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = -1
	return proxy
}

func compileRuntime(app *App, assets fs.FS, manifest *compiledManifest, render renderer) (*runtimeState, error) {
	if app == nil || manifest == nil {
		return nil, errors.New("bifrost: incomplete runtime input")
	}
	state := &runtimeState{
		assets:         assets,
		manifest:       manifest,
		renderer:       render,
		handlers:       make(map[string]http.Handler, len(app.routes)),
		serverPatterns: make(map[string]struct{}),
		files:          make(map[string]protocol.FileRef),
		public:         manifest.public,
	}
	maps.Copy(state.files, manifest.clientFiles)
	for _, route := range manifest.routes {
		for _, document := range route.Documents {
			state.files[document.File.Path] = document.File
		}
	}

	declarations := make(map[string]Route, len(app.routes))
	for _, route := range app.routes {
		declarations[route.pattern] = route
	}
	devDir := os.Getenv("BIFROST_DEV_DIR")
	if devDir != "" {
		port, portErr := strconv.Atoi(os.Getenv("BIFROST_VITE_PORT"))
		if portErr != nil || port < 1 || port > 65535 {
			return nil, errors.New("bifrost: invalid BIFROST_VITE_PORT")
		}
		state.devProxy = newDevelopmentProxy(port)
	}
	for pattern, builtRoute := range manifest.routes {
		declaration := declarations[pattern]
		view := manifest.views[builtRoute.ViewID]
		clientAssets := view.Client
		serverEntry := ""
		if view.Server != nil {
			serverEntry = view.Server.Entry.Path
		}
		if devDir != "" {
			clientFile := filepath.Join(devDir, "entries", view.ID+"-client.tsx")
			clientModule := (&url.URL{Path: dochtml.DevPrefix + "@fs" + filepath.ToSlash(clientFile)}).EscapedPath()
			clientAssets = protocol.AssetSet{Entry: protocol.FileRef{Path: clientModule}}
			if view.Mode == "hydrate" {
				serverFile := filepath.Join(devDir, "entries", view.ID+"-server.tsx")
				serverEntry = (&url.URL{Path: "/@fs" + filepath.ToSlash(serverFile)}).EscapedPath()
			}
		}
		shell, err := dochtml.NewShell(clientAssets)
		if err != nil {
			return nil, fmt.Errorf("bifrost: compile route %q shell: %w", pattern, err)
		}

		var handler http.Handler
		switch declaration.kind {
		case routeServer:
			if render == nil || serverEntry == "" {
				return nil, fmt.Errorf("bifrost: server route %q has no renderer", pattern)
			}
			state.serverPatterns["GET "+pattern] = struct{}{}
			handler = &serverPageHandler{
				pattern: pattern,
				load:    declaration.load,
				entry:   serverEntry,
				shell:   shell,
				render:  render,
				hooks:   app.hooks,
				limits:  app.limits,
				logger:  app.logger,
			}
		case routeStatic:
			if devDir != "" {
				if render == nil || serverEntry == "" {
					return nil, fmt.Errorf("bifrost: static development route %q has no renderer", pattern)
				}
				pages := make(map[string]developmentStaticPage, len(builtRoute.Documents))
				for _, document := range builtRoute.Documents {
					props := document.Props
					if len(props) == 0 {
						props = emptyProps
					}
					pages[document.Path] = developmentStaticPage{props: props, document: documentFromProtocol(document.Document)}
				}
				handler = &developmentStaticHandler{pages: pages, page: &serverPageHandler{pattern: pattern, entry: serverEntry, shell: shell, render: render, hooks: app.hooks, limits: app.limits, logger: app.logger}}
			} else {
				files := make(map[string]protocol.FileRef, len(builtRoute.Documents))
				for _, document := range builtRoute.Documents {
					files[document.Path] = document.File
				}
				handler = &staticPageHandler{assets: assets, files: files, headers: app.hooks.assetHeaders}
			}
		case routeClient:
			document, err := shell.ClientDocument(emptyProps)
			if err != nil {
				return nil, fmt.Errorf("bifrost: compile client route %q: %w", pattern, err)
			}
			handler = &clientPageHandler{document: document, contentLength: strconv.Itoa(len(document))}
		default:
			return nil, fmt.Errorf("bifrost: route %q has invalid kind", pattern)
		}

		if len(app.hooks.responseHooks) > 0 {
			handler = &responseObserver{pattern: pattern, next: handler, hooks: app.hooks.responseHooks}
		}
		for i := len(app.hooks.middleware) - 1; i >= 0; i-- {
			handler = app.hooks.middleware[i](handler)
			if handler == nil {
				return nil, fmt.Errorf("bifrost: middleware %d returned nil for route %q", i, pattern)
			}
		}
		state.handlers[pattern] = handler
	}
	return state, nil
}

type serverPageHandler struct {
	pattern string
	load    Loader
	entry   string
	shell   dochtml.Shell
	render  renderer
	hooks   registeredHooks
	limits  Limits
	logger  *slog.Logger
}

func (h *serverPageHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	props := any(nil)
	if h.load != nil {
		var started time.Time
		if len(h.hooks.loadHooks) > 0 {
			started = time.Now()
		}
		loaded, err := h.load(request)
		if len(h.hooks.loadHooks) > 0 {
			event := LoadEvent{Pattern: h.pattern, Duration: time.Since(started), Err: err}
			for _, hook := range h.hooks.loadHooks {
				hook(request.Context(), event)
			}
		}
		if err != nil {
			h.serveError(w, request, err)
			return
		}
		props = loaded
	}

	status := http.StatusOK
	errorFallbacks := 0
	if data, ok := props.(PageData); ok {
		errorFallbacks = data.ErrorFallbacks
		if data.Status != 0 {
			status = data.Status
			if status < 200 || status > 599 {
				h.serveError(w, request, fmt.Errorf("invalid page status %d", status))
				return
			}
		}
	}
	props, document, err := splitPageData(props)
	if err != nil {
		h.serveError(w, request, fmt.Errorf("encode document metadata: %w", err))
		return
	}
	propsJSON, err := marshalProps(props)
	if err == nil && len(propsJSON) > h.limits.MaxPropsBytes {
		err = fmt.Errorf("page props exceed %d bytes", h.limits.MaxPropsBytes)
	}
	if err != nil {
		h.serveError(w, request, fmt.Errorf("encode page props: %w", err))
		return
	}

	h.renderProps(w, request, propsJSON, document, status, errorFallbacks)
}

func (h *serverPageHandler) renderProps(w http.ResponseWriter, request *http.Request, propsJSON json.RawMessage, document Document, status, errorFallbacks int) {
	var sink pageRenderSink
	if markdownRequested(request.Context()) {
		sink = &markdownRenderSink{writer: w, limits: h.limits}
	} else {
		sink = &httpRenderSink{writer: w, shell: h.shell, props: propsJSON, document: document, status: status, limits: h.limits}
	}
	var started time.Time
	if len(h.hooks.renderHooks) > 0 {
		started = time.Now()
	}
	err := h.render.Render(request.Context(), renderRequest{Pattern: h.pattern, Entry: h.entry, Props: propsJSON}, sink)
	if err == nil {
		err = sink.finish()
	}
	if len(h.hooks.renderHooks) > 0 {
		event := RenderEvent{Pattern: h.pattern, Duration: time.Since(started), Err: err}
		for _, hook := range h.hooks.renderHooks {
			hook(request.Context(), event)
		}
	}
	if err != nil {
		if !sink.committed() && errorFallbacks > 0 {
			if h.renderError(w, request, document, err, errorFallbacks) {
				return
			}
		}
		if !sink.committed() {
			h.serveError(w, request, err)
		} else if h.logger != nil {
			h.logger.Error("bifrost render failed after response commit", "pattern", h.pattern, "error", err)
		}
	}
}

func (h *serverPageHandler) renderError(w http.ResponseWriter, request *http.Request, document Document, renderErr error, fallbacks int) bool {
	message := http.StatusText(http.StatusInternalServerError)
	if os.Getenv("BIFROST_DEV_DIR") != "" {
		message = renderErr.Error()
	}
	for level := fallbacks - 1; level >= 0; level-- {
		props, err := marshalProps(map[string]any{"__bifrostError": message, "__bifrostErrorLevel": level})
		if err != nil {
			return false
		}
		sink := &httpRenderSink{writer: w, shell: h.shell, props: props, document: document, status: http.StatusInternalServerError, limits: h.limits}
		err = h.render.Render(request.Context(), renderRequest{Pattern: h.pattern, Entry: h.entry, Props: props}, sink)
		if err == nil {
			err = sink.finish()
		}
		if err == nil {
			return true
		}
		if sink.committed() {
			if h.logger != nil {
				h.logger.Error("bifrost error page failed after response commit", "pattern", h.pattern, "error", err)
			}
			return true
		}
	}
	return false
}

type developmentStaticPage struct {
	props    json.RawMessage
	document Document
}

type developmentStaticHandler struct {
	pages map[string]developmentStaticPage
	page  *serverPageHandler
}

func (h *developmentStaticHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	page, exists := h.pages[request.URL.Path]
	if !exists {
		http.NotFound(w, request)
		return
	}
	h.page.renderProps(w, request, page.props, page.document, http.StatusOK, 0)
}

func (h *serverPageHandler) serveError(w http.ResponseWriter, request *http.Request, err error) {
	if h.hooks.errorHandler != nil {
		h.hooks.errorHandler(w, request, err)
		return
	}
	serveDefaultError(w, request, err)
}

type httpRenderSink struct {
	writer   http.ResponseWriter
	shell    dochtml.Shell
	props    json.RawMessage
	document Document
	status   int
	started  bool
	finished bool
	limits   Limits
}

func (s *httpRenderSink) Head(head []byte) error {
	if len(head) > s.limits.MaxHeadBytes {
		return fmt.Errorf("renderer head exceeds %d bytes", s.limits.MaxHeadBytes)
	}
	if s.started {
		return errors.New("bifrost: renderer emitted head more than once")
	}
	s.writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	s.writer.Header().Set("Cache-Control", "no-store")
	s.writer.WriteHeader(s.status)
	s.started = true
	if err := s.shell.WritePreamble(s.writer, head, protocolDocument(s.document)); err != nil {
		return err
	}
	if flusher, ok := s.writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func (s *httpRenderSink) Body(body []byte) error {
	if len(body) > s.limits.MaxFrameBytes {
		return fmt.Errorf("renderer frame exceeds %d bytes", s.limits.MaxFrameBytes)
	}
	if !s.started {
		return errors.New("bifrost: renderer emitted body before head")
	}
	if s.finished {
		return errors.New("bifrost: renderer emitted body after completion")
	}
	_, err := s.writer.Write(body)
	return err
}

func (s *httpRenderSink) committed() bool { return s.started }

func (s *httpRenderSink) finish() error {
	if !s.started {
		return errors.New("bifrost: renderer did not emit head")
	}
	if s.finished {
		return errors.New("bifrost: renderer completed more than once")
	}
	s.finished = true
	return s.shell.WriteSuffix(s.writer, s.props)
}

type staticPageHandler struct {
	assets  fs.FS
	files   map[string]protocol.FileRef
	headers AssetHeaderHook
}

func (h *staticPageHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	file, exists := h.files[request.URL.Path]
	if !exists {
		http.NotFound(w, request)
		return
	}
	serveArtifact(w, request, h.assets, file, "text/html; charset=utf-8", "no-cache", h.headers, false)
}

type clientPageHandler struct {
	document      []byte
	contentLength string
}

func (h *clientPageHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Length", h.contentLength)
	w.WriteHeader(http.StatusOK)
	if request.Method != http.MethodHead {
		_, _ = w.Write(h.document)
	}
}

type publicAssetHandler struct {
	assets  fs.FS
	file    protocol.FileRef
	headers AssetHeaderHook
}

func (h *publicAssetHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	extension := path.Ext(h.file.Path)
	contentType := mime.TypeByExtension(extension)
	if extension == ".webmanifest" {
		contentType = "application/manifest+json"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	serveArtifact(w, request, h.assets, h.file, contentType, "public, max-age=3600", h.headers, true)
}

type assetHandler struct {
	assets  fs.FS
	files   map[string]protocol.FileRef
	headers AssetHeaderHook
}

func (h *assetHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	filePath := strings.TrimPrefix(request.URL.Path, dochtml.AssetPrefix)
	file, exists := h.files[filePath]
	if !exists {
		http.NotFound(w, request)
		return
	}
	contentType := mime.TypeByExtension(path.Ext(filePath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	serveArtifact(w, request, h.assets, file, contentType, "public, max-age=31536000, immutable", h.headers, false)
}

var artifactBuffers = sync.Pool{New: func() any {
	buffer := make([]byte, 32<<10)
	return &buffer
}}

func serveArtifact(w http.ResponseWriter, request *http.Request, assets fs.FS, file protocol.FileRef, contentType, cacheControl string, headers AssetHeaderHook, public bool) {
	etag := `"` + file.Hash + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("Content-Type", contentType)
	if headers != nil {
		headers(w.Header(), public)
	}
	if request.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	opened, err := assets.Open(file.Path)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	defer func() { _ = opened.Close() }()
	w.Header().Set("Content-Length", fmt.Sprintf("%d", file.Size))
	w.WriteHeader(http.StatusOK)
	if request.Method != http.MethodHead {
		buffer := artifactBuffers.Get().(*[]byte)
		_, _ = io.CopyBuffer(w, opened, *buffer)
		artifactBuffers.Put(buffer)
	}
}

type responseObserver struct {
	pattern string
	next    http.Handler
	hooks   []ResponseHook
}

func (h *responseObserver) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	writer := &observedWriter{ResponseWriter: w, status: http.StatusOK}
	started := time.Now()
	h.next.ServeHTTP(writer, request)
	event := ResponseEvent{
		Pattern:  h.pattern,
		Status:   writer.status,
		Bytes:    writer.bytes,
		Duration: time.Since(started),
		Err:      writer.err,
	}
	for _, hook := range h.hooks {
		hook(request.Context(), event)
	}
}

type observedWriter struct {
	http.ResponseWriter
	status      int
	bytes       int64
	err         error
	wroteHeader bool
}

func (w *observedWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *observedWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	count, err := w.ResponseWriter.Write(data)
	w.bytes += int64(count)
	if err != nil && w.err == nil {
		w.err = err
	}
	return count, err
}

func (w *observedWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *observedWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
