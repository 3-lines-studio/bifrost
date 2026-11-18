package bifrost

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/3-lines-studio/bifrost/internal/dochtml"
	"github.com/3-lines-studio/bifrost/internal/protocol"
)

// htmlPageHandler answers one HTML route: load, render templates, write the
// document. There is no renderer process, no props payload, and no hydration.
type htmlPageHandler struct {
	pattern      string
	page         HTMLPage
	templates    fs.FS
	assets       protocol.AssetSet
	layoutAssets protocol.AssetSet
	development  bool
	hooks        registeredHooks
	limits       Limits
	logger       *slog.Logger
	cacheControl string

	once     sync.Once
	parsed   *htmlTemplates
	parseErr error
}

type htmlTemplates struct {
	page    *template.Template
	layouts []*template.Template
}

func (h *htmlPageHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	data, document, status, err := h.load(request)
	if err != nil {
		h.serveError(w, request, err)
		return
	}
	chrome, chromeDocument, _, err := h.loadLayout(request)
	if err != nil {
		h.serveError(w, request, err)
		return
	}
	// The chrome knows the language of the request, so it is the layout that
	// declares it; a page that sets its own document attributes keeps them.
	document = preferDocument(document, chromeDocument)

	head, err := h.head(request)
	if err != nil {
		h.serveError(w, request, fmt.Errorf("html metadata: %w", err))
		return
	}

	body, err := h.render(request, data, chrome)
	if err != nil {
		h.serveError(w, request, err)
		return
	}

	if markdownRequested(request.Context()) {
		if err := serveMarkdown(w, body, status, h.limits); err != nil {
			h.serveError(w, request, err)
		}
		return
	}

	var out bytes.Buffer
	h.writeDocument(&out, document, head, body)
	h.writeResponse(w, out.Bytes(), status)
}

func (h *htmlPageHandler) load(request *http.Request) (any, Document, int, error) {
	if h.page.Load == nil {
		document, err := normalizeDocument(Document{})
		return nil, document, http.StatusOK, err
	}
	return runLoader(h.hooks.loadHooks, h.pattern, h.page.Load, request)
}

func runLoader(hooks []LoadHook, name string, load Loader, request *http.Request) (any, Document, int, error) {
	started := time.Now()
	loaded, err := load(request)
	if len(hooks) > 0 {
		event := LoadEvent{Pattern: name, Duration: time.Since(started), Err: err}
		for _, hook := range hooks {
			hook(request.Context(), event)
		}
	}
	if err != nil {
		return nil, Document{}, 0, err
	}

	status := http.StatusOK
	if data, ok := loaded.(PageData); ok {
		if data.Status != 0 {
			status = data.Status
			if status < 200 || status > 599 {
				return nil, Document{}, 0, fmt.Errorf("invalid page status %d", status)
			}
		}
	}
	props, document, err := splitPageData(loaded)
	if err != nil {
		return nil, Document{}, 0, fmt.Errorf("encode document metadata: %w", err)
	}
	return props, document, status, nil
}

func (h *htmlPageHandler) loadLayout(request *http.Request) (any, Document, int, error) {
	if h.page.LayoutData == nil {
		return nil, Document{}, http.StatusOK, nil
	}
	return runLoader(h.hooks.loadHooks, h.pattern+" layout", h.page.LayoutData, request)
}

func (h *htmlPageHandler) head(request *http.Request) ([]byte, error) {
	layout, err := metadataOf(h.page.LayoutMetadata, request)
	if err != nil {
		return nil, err
	}
	page, err := metadataOf(h.page.Metadata, request)
	if err != nil {
		return nil, err
	}
	return renderMetadata(mergeMetadata(layout, page))
}

func metadataOf(metadata MetadataFunc, request *http.Request) (Metadata, error) {
	if metadata == nil {
		return Metadata{}, nil
	}
	return metadata(request)
}

// mergeMetadata lets the chrome set what every page shares and the page refine
// it: an empty page field keeps the layout's, and both Head fragments survive so
// neither side loses the tags only it knows about.
func mergeMetadata(layout, page Metadata) Metadata {
	merged := page
	for _, field := range []struct {
		target *string
		page   string
		layout string
	}{
		{&merged.Title, page.Title, layout.Title},
		{&merged.Description, page.Description, layout.Description},
		{&merged.Keywords, page.Keywords, layout.Keywords},
		{&merged.Robots, page.Robots, layout.Robots},
		{&merged.Canonical, page.Canonical, layout.Canonical},
		{&merged.Image, page.Image, layout.Image},
		{&merged.ImageAlt, page.ImageAlt, layout.ImageAlt},
		{&merged.Type, page.Type, layout.Type},
		{&merged.Locale, page.Locale, layout.Locale},
	} {
		if field.page == "" {
			*field.target = field.layout
		}
	}
	merged.Head = layout.Head + page.Head
	return merged
}

func (h *htmlPageHandler) templatesFor() (*htmlTemplates, error) {
	if h.development {
		return h.parse()
	}
	h.once.Do(func() { h.parsed, h.parseErr = h.parse() })
	return h.parsed, h.parseErr
}

func (h *htmlPageHandler) parse() (*htmlTemplates, error) {
	parsed := &htmlTemplates{}
	page, err := h.parseTemplate(h.page.Page)
	if err != nil {
		return nil, err
	}
	parsed.page = page
	for _, layout := range h.page.Layouts {
		parsedLayout, err := h.parseTemplate(layout)
		if err != nil {
			return nil, err
		}
		parsed.layouts = append(parsed.layouts, parsedLayout)
	}
	return parsed, nil
}

func (h *htmlPageHandler) parseTemplate(name string) (*template.Template, error) {
	if h.templates == nil {
		return nil, errors.New("templates: Config.Templates is missing")
	}
	file, err := h.templates.Open(name)
	if err != nil {
		return nil, fmt.Errorf("templates: open %s: %w", name, err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("templates: read %s: %w", name, err)
	}
	parsed, err := template.New(path.Base(name)).Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("templates: parse %s: %w", name, err)
	}
	return parsed, nil
}

func (h *htmlPageHandler) render(request *http.Request, data, chrome any) ([]byte, error) {
	parsed, err := h.templatesFor()
	if err != nil {
		return nil, err
	}
	var body bytes.Buffer
	if err := parsed.page.Execute(&body, data); err != nil {
		return nil, fmt.Errorf("templates: render %s: %w", h.page.Page, err)
	}
	children := template.HTML(body.String())
	frame := Frame{
		Data:   chrome,
		Path:   request.URL.EscapedPath(),
		Query:  request.URL.Query(),
		Params: h.params(request),
	}
	for index := len(parsed.layouts) - 1; index >= 0; index-- {
		frame.Children = children
		var rendered bytes.Buffer
		if err := parsed.layouts[index].Execute(&rendered, frame); err != nil {
			return nil, fmt.Errorf("templates: render %s: %w", h.page.Layouts[index], err)
		}
		children = template.HTML(rendered.String())
	}
	return []byte(children), nil
}

func (h *htmlPageHandler) params(request *http.Request) map[string]string {
	if len(h.page.Params) == 0 {
		return nil
	}
	values := make(map[string]string, len(h.page.Params))
	for _, name := range h.page.Params {
		values[name] = request.PathValue(name)
	}
	return values
}

// writeDocument assembles the document around the rendered body. The head holds
// the metadata, the styles of the route, and the module the route brings; the
// body holds nothing but what the templates rendered.
func (h *htmlPageHandler) writeDocument(w io.Writer, document Document, head, body []byte) {
	var out strings.Builder
	out.WriteString(dochtml.Open(protocol.DocumentAttributes{Lang: document.Lang, Class: document.Class, Dir: document.Dir}))
	out.Write(head)
	// The chrome first: its styles are the ones a page refines, and its script is
	// the theme switch a page never replaces.
	for _, style := range h.layoutAssets.Styles {
		out.WriteString(`<link rel="stylesheet" href="`)
		out.WriteString(html.EscapeString(dochtml.FrontendAssetURL(style.Path)))
		out.WriteString(`">`)
	}
	for _, style := range h.assets.Styles {
		out.WriteString(`<link rel="stylesheet" href="`)
		out.WriteString(html.EscapeString(dochtml.FrontendAssetURL(style.Path)))
		out.WriteString(`">`)
	}
	out.WriteString(`</head><body>`)
	out.Write(body)
	if entry := h.layoutAssets.Entry.Path; entry != "" {
		out.WriteString(`<script type="module" src="`)
		out.WriteString(html.EscapeString(dochtml.FrontendAssetURL(entry)))
		out.WriteString(`"></script>`)
	}
	if entry := h.assets.Entry.Path; entry != "" {
		out.WriteString(`<script type="module" src="`)
		out.WriteString(html.EscapeString(dochtml.FrontendAssetURL(entry)))
		out.WriteString(`"></script>`)
	}
	out.WriteString(`</body></html>`)
	io.WriteString(w, out.String())
}

func (h *htmlPageHandler) writeResponse(w http.ResponseWriter, document []byte, status int) {
	header := w.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Cache-Control", cacheControlOr(h.cacheControl, defaultDocumentCacheControl))
	header.Set("Content-Length", fmt.Sprintf("%d", len(document)))
	w.WriteHeader(status)
	_, _ = w.Write(document)
}

func (h *htmlPageHandler) serveError(w http.ResponseWriter, request *http.Request, err error) {
	if request.Context().Err() == context.Canceled && errors.Is(err, context.Canceled) {
		w.WriteHeader(statusClientClosedRequest)
		return
	}
	if h.hooks.errorHandler != nil {
		h.hooks.errorHandler(w, request, err)
		return
	}
	serveDefaultError(w, request, err)
}

// preferDocument keeps the page's document attributes, and takes the layout's
// when the page left them at the default.
func preferDocument(page, layout Document) Document {
	if page != (Document{Lang: "en"}) || layout == (Document{Lang: "en"}) {
		return page
	}
	return layout
}

func cacheControlOr(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

// renderMetadata writes the head of an HTML page. Every field except Head is
// escaped; Head is the app's own markup.
func renderMetadata(metadata Metadata) ([]byte, error) {
	var out bytes.Buffer
	writeTag := func(name, value string) {
		if value == "" {
			return
		}
		out.WriteString(`<meta name="`)
		out.WriteString(html.EscapeString(name))
		out.WriteString(`" content="`)
		out.WriteString(html.EscapeString(value))
		out.WriteString(`">`)
	}
	writeProperty := func(name, value string) {
		if value == "" {
			return
		}
		out.WriteString(`<meta property="`)
		out.WriteString(html.EscapeString(name))
		out.WriteString(`" content="`)
		out.WriteString(html.EscapeString(value))
		out.WriteString(`">`)
	}
	if metadata.Title != "" {
		out.WriteString("<title>")
		out.WriteString(html.EscapeString(metadata.Title))
		out.WriteString("</title>")
		writeTag("title", metadata.Title)
	}
	writeTag("description", metadata.Description)
	writeTag("keywords", metadata.Keywords)
	writeTag("robots", metadata.Robots)
	if metadata.Canonical != "" {
		out.WriteString(`<link rel="canonical" href="`)
		out.WriteString(html.EscapeString(metadata.Canonical))
		out.WriteString(`">`)
	}
	writeProperty("og:title", metadata.Title)
	writeProperty("og:description", metadata.Description)
	writeProperty("og:url", metadata.Canonical)
	writeProperty("og:type", metadata.Type)
	writeProperty("og:locale", metadata.Locale)
	writeProperty("og:image", metadata.Image)
	writeProperty("og:image:alt", metadata.ImageAlt)
	out.WriteString(string(metadata.Head))
	return out.Bytes(), nil
}
