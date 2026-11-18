package bifrost

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type nativePageHandler struct {
	pattern      string
	render       NativeRender
	errorRender  NativeError
	load         Loader
	limits       Limits
	hooks        registeredHooks
	cacheControl string
}

func (a *App) initializeNative() error {
	runtime := &runtimeState{
		handlers:       make(map[string]http.Handler, len(a.routes)),
		serverPatterns: make(map[string]struct{}),
	}
	if err := a.addNativeHandlers(runtime); err != nil {
		return err
	}
	a.runtime = runtime
	if err := a.Register(http.NewServeMux()); err != nil {
		a.runtime = nil
		return err
	}
	return nil
}

func (a *App) addNativeHandlers(runtime *runtimeState) error {
	for _, route := range a.routes {
		if route.kind != routeNative {
			continue
		}
		handler, err := a.nativeHandler(route)
		if err != nil {
			return err
		}
		runtime.handlers[route.pattern] = handler
	}
	return nil
}

func (a *App) nativeHandler(route Route) (http.Handler, error) {
	var page http.Handler = &nativePageHandler{
		pattern:      route.pattern,
		render:       route.render,
		errorRender:  route.errorRender,
		load:         route.load,
		limits:       a.limits,
		hooks:        a.hooks,
		cacheControl: route.cacheControl,
	}
	if len(a.hooks.responseHooks) > 0 {
		page = &responseObserver{pattern: route.pattern, next: page, hooks: a.hooks.responseHooks}
	}
	for index := len(a.hooks.middleware) - 1; index >= 0; index-- {
		page = a.hooks.middleware[index](page)
		if page == nil {
			return nil, fmt.Errorf("bifrost: middleware %d returned nil for route %q", index, route.pattern)
		}
	}
	return page, nil
}

func (h *nativePageHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	props, err := h.loadProps(request)
	if err != nil {
		h.serveLoadError(w, request, err)
		return
	}

	status := http.StatusOK
	if data, ok := props.(PageData); ok && data.Status != 0 {
		status = data.Status
		if status < 200 || status > 599 {
			h.serveError(w, request, fmt.Errorf("invalid page status %d", status))
			return
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

	page, err := h.render(request.Context(), propsJSON)
	if err != nil {
		h.serveError(w, request, err)
		return
	}
	if err := h.writeDocument(w, request, status, h.cacheControl, document, page); err != nil {
		h.serveError(w, request, err)
	}
}

// writeDocument composes and writes a Native page with its status. The caller
// writes the error response when it fails.
func (h *nativePageHandler) writeDocument(w http.ResponseWriter, request *http.Request, status int, cacheControl string, document Document, page NativePage) error {
	if len(page.Body) > h.limits.MaxFrameBytes {
		return fmt.Errorf("native page body exceeds %d bytes", h.limits.MaxFrameBytes)
	}
	head, tail, err := composeNativeDocument(document, page, h.limits)
	if err != nil {
		return err
	}
	if cacheControl == "" {
		cacheControl = defaultDocumentCacheControl
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("Content-Length", strconv.Itoa(len(head)+len(page.Body)+len(tail)))
	w.WriteHeader(status)
	if request.Method == http.MethodHead {
		return nil
	}
	_, _ = w.Write(head)
	_, _ = w.Write(page.Body)
	_, _ = w.Write(tail)
	return nil
}

// serveLoadError renders the route's error page for a loader that failed, or
// lets the default error response or a hook answer it.
func (h *nativePageHandler) serveLoadError(w http.ResponseWriter, request *http.Request, cause error) {
	if h.errorRender == nil || h.hooks.errorHandler != nil || IsRedirect(cause) || request.Context().Err() != nil {
		h.serveError(w, request, cause)
		return
	}
	status, ok := ErrorStatus(cause)
	if !ok || status < 400 || status > 599 {
		status = http.StatusInternalServerError
	}
	page, err := h.errorRender(request.Context(), cause)
	if err != nil {
		h.serveError(w, request, fmt.Errorf("render error page: %w", err))
		return
	}
	document, err := normalizeDocument(Document{})
	if err != nil {
		h.serveError(w, request, err)
		return
	}
	if err := h.writeDocument(w, request, status, defaultDocumentCacheControl, document, page); err != nil {
		h.serveError(w, request, err)
	}
}

func (h *nativePageHandler) loadProps(request *http.Request) (any, error) {
	if h.load == nil {
		return nil, nil
	}
	started := time.Now()
	props, err := h.load(request)
	if len(h.hooks.loadHooks) > 0 {
		event := LoadEvent{Pattern: h.pattern, Duration: time.Since(started), Err: err}
		for _, hook := range h.hooks.loadHooks {
			hook(request.Context(), event)
		}
	}
	return props, err
}

func (h *nativePageHandler) serveError(w http.ResponseWriter, request *http.Request, err error) {
	serveRouteError(w, request, h.hooks, err)
}

func composeNativeDocument(document Document, page NativePage, limits Limits) (head, tail []byte, err error) {
	if page.Lang != "" {
		if !validLanguageTag(page.Lang) {
			return nil, nil, fmt.Errorf("native page has an invalid language")
		}
		document.Lang = page.Lang
	}
	if page.Dir != "" {
		if page.Dir != "ltr" && page.Dir != "rtl" && page.Dir != "auto" {
			return nil, nil, fmt.Errorf("native page direction must be ltr, rtl, or auto")
		}
		document.Dir = page.Dir
	}
	if page.Schema != "" && !json.Valid([]byte(page.Schema)) {
		return nil, nil, fmt.Errorf("native page schema is not JSON")
	}
	if err := validateAlternates(page.Alternates); err != nil {
		return nil, nil, err
	}

	var out strings.Builder
	out.WriteString(`<!doctype html><html lang="`)
	out.WriteString(html.EscapeString(document.Lang))
	out.WriteString(`"`)
	if document.Dir != "" {
		out.WriteString(` dir="`)
		out.WriteString(html.EscapeString(document.Dir))
		out.WriteString(`"`)
	}
	if document.Class != "" {
		out.WriteString(` class="`)
		out.WriteString(html.EscapeString(document.Class))
		out.WriteString(`"`)
	}
	out.WriteString(`><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">`)
	if page.Title != "" {
		writeTitle(&out, page.Title)
	}
	if page.Description != "" {
		writeMeta(&out, `name="description" content="`, page.Description)
		writeMeta(&out, `property="og:description" content="`, page.Description)
	}
	if page.Title != "" {
		writeMeta(&out, `property="og:title" content="`, page.Title)
	}
	if page.Canonical != "" {
		out.WriteString(`<link rel="canonical" href="`)
		out.WriteString(html.EscapeString(page.Canonical))
		out.WriteString(`">`)
		writeMeta(&out, `property="og:url" content="`, page.Canonical)
	}
	for _, alternate := range page.Alternates {
		out.WriteString(`<link rel="alternate" hreflang="`)
		out.WriteString(html.EscapeString(alternate.Lang))
		out.WriteString(`" href="`)
		out.WriteString(html.EscapeString(alternate.URL))
		out.WriteString(`">`)
	}
	if page.Image != "" {
		writeMeta(&out, `property="og:image" content="`, page.Image)
		writeMeta(&out, `name="twitter:card" content="`, "summary_large_image")
	}
	if page.Type != "" || page.Title != "" {
		kind := page.Type
		if kind == "" {
			kind = "website"
		}
		writeMeta(&out, `property="og:type" content="`, kind)
	}
	if page.Keywords != "" {
		writeMeta(&out, `name="keywords" content="`, page.Keywords)
	}
	if page.Robots != "" {
		writeMeta(&out, `name="robots" content="`, page.Robots)
	}
	writeSchema(&out, page.Schema)
	out.WriteString(`</head><body>`)
	if out.Len() > limits.MaxHeadBytes {
		return nil, nil, fmt.Errorf("native page head exceeds %d bytes", limits.MaxHeadBytes)
	}
	return []byte(out.String()), []byte(`</body></html>`), nil
}

// writeSchema writes the JSON-LD of a page: one value becomes one script, and
// an array of them becomes one script each, so a page can describe itself as a
// graph of things a search engine reads apart.
func writeSchema(out *strings.Builder, schema string) {
	if schema == "" {
		return
	}
	values := []string{schema}
	if strings.HasPrefix(schema, `[`) {
		var array []json.RawMessage
		if err := json.Unmarshal([]byte(schema), &array); err == nil && len(array) > 0 {
			values = values[:0]
			for _, value := range array {
				values = append(values, string(value))
			}
		}
	}
	for _, value := range values {
		out.WriteString(`<script type="application/ld+json">`)
		out.WriteString(escapeScript(value))
		out.WriteString(`</script>`)
	}
}

// validateAlternates keeps a malformed language tag or URL out of the document.
func validateAlternates(alternates []Alternate) error {
	for _, alternate := range alternates {
		if alternate.Lang != "x-default" && !validLanguageTag(alternate.Lang) {
			return fmt.Errorf("native page has an invalid alternate language")
		}
		if alternate.URL == "" {
			return fmt.Errorf("native page alternate %q has no URL", alternate.Lang)
		}
	}
	return nil
}

func writeTitle(out *strings.Builder, title string) {
	out.WriteString(`<title>`)
	out.WriteString(html.EscapeString(title))
	out.WriteString(`</title>`)
}

func writeMeta(out *strings.Builder, attribute, value string) {
	out.WriteString(`<meta `)
	out.WriteString(attribute)
	out.WriteString(html.EscapeString(value))
	out.WriteString(`">`)
}

// escapeScript keeps a JSON-LD value from closing its script element.
func escapeScript(value string) string {
	value = strings.ReplaceAll(value, "<", `\u003c`)
	value = strings.ReplaceAll(value, ">", `\u003e`)
	return strings.ReplaceAll(value, "&", `\u0026`)
}
