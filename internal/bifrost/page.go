package bifrost

import (
	"bytes"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type Page struct {
	renderer    *Renderer
	opts        options
	propsLoader propsLoader
	entryDir    string
	outdir      string
	entryPath   string
	entryName   string
	scriptSrc   string
	cssHref     string
	chunks      []string
	manifest    *buildManifest
	isDev       bool
	needsSetup  bool
	setupErr    error
	setupOnce   sync.Once
}

func (p *Page) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if p.needsSetup {
		if !p.handleSetup(w) {
			return
		}
	}

	var loaderStart time.Time
	if p.renderer.timingEnabled {
		loaderStart = time.Now()
	}

	props, err := p.propsLoader(req)
	if err != nil {
		p.handlePropsError(w, req, err)
		return
	}

	if p.renderer.timingEnabled {
		loaderDuration := time.Since(loaderStart)
		slog.Debug("data loader timing", "duration", loaderDuration)
	}

	var renderStart time.Time
	if p.renderer.timingEnabled {
		renderStart = time.Now()
	}

	page, err := p.renderer.Render(p.opts.ComponentPath, props)
	if err != nil {
		p.serveError(w, err)
		return
	}

	if p.renderer.timingEnabled {
		renderDuration := time.Since(renderStart)
		slog.Debug("ssr render timing", "duration", renderDuration)
	}

	p.renderPage(w, props, page)
}

func (p *Page) handleSetup(w http.ResponseWriter) bool {
	p.setupOnce.Do(func() {
		p.setupErr = p.renderer.setupPage(p.opts, p.entryDir, p.outdir, p.entryPath)
	})
	return p.checkSetupError(w)
}

func (p *Page) checkSetupError(w http.ResponseWriter) bool {
	if p.setupErr != nil {
		p.serveError(w, p.setupErr)
		return false
	}
	return true
}

func (p *Page) handlePropsError(w http.ResponseWriter, req *http.Request, err error) {
	redirectErr, isRedirect := err.(RedirectError)
	if !isRedirect {
		p.serveError(w, err)
		return
	}

	status := redirectErr.RedirectStatusCode()
	if status == 0 {
		status = http.StatusFound
	}
	http.Redirect(w, req, redirectErr.RedirectURL(), status)
}

func (p *Page) renderPage(w http.ResponseWriter, props map[string]interface{}, page renderedPage) {
	finalScript := p.scriptSrc
	finalCSS := p.cssHref

	if p.opts.Watch {
		stamp := fmt.Sprintf("%d", time.Now().UnixNano())
		finalScript = addCacheBust(finalScript, stamp)
		finalCSS = addCacheBust(finalCSS, stamp)
	}

	fullHTML, err := htmlShell(page.Body, props, finalScript, p.opts.Title, page.Head, finalCSS, p.chunks)
	if err != nil {
		p.serveError(w, err)
		return
	}

	if p.opts.Watch {
		fullHTML = appendReloadScript(fullHTML)
	}

	serveHTML(w, fullHTML)
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

func (p *Page) serveError(w http.ResponseWriter, err error) {
	data := errorData{
		Message: "Internal Server Error",
		IsDev:   p.isDev,
	}

	if bifrostErr, ok := err.(*BifrostError); ok {
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
