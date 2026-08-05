package http

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"

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
	shell           *core.HTMLDocumentShell
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
	entryName := core.EntryNameForPath(config.ComponentPath)
	artifacts := core.ResolvePageArtifacts(manifest, entryName)
	var shell *core.HTMLDocumentShell
	if builtShell, err := core.NewHTMLDocumentShell(
		artifacts.Script,
		artifacts.CriticalCSS,
		core.StylesheetHrefsFor(artifacts),
		artifacts.Chunks,
	); err == nil {
		shell = &builtShell
	}

	return &PageHandler{
		service:         service,
		config:          config,
		manifest:        manifest,
		assetsFS:        assetsFS,
		isDev:           isDev,
		entryName:       entryName,
		staticPath:      staticPath,
		defaultHTMLLang: defaultHTMLLang,
		shell:           shell,
	}
}

var errNeedsSetup = errors.New("page needs setup but setup not implemented in adapter")

func (h *PageHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	output := h.service.ServePage(req.Context(), h.servePageInput(req))
	if output.Error != nil {
		h.serveError(w, req, output.Error)
		return
	}
	h.dispatchPageOutput(w, req, output)
}

type markdownCtxKey struct{}

func ResolveMarkdown(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		markdown := false
		if strings.HasSuffix(strings.ToLower(req.URL.Path), ".md") {
			req = req.Clone(req.Context())
			req.URL.Path = req.URL.Path[:len(req.URL.Path)-3]
			markdown = true
		}
		if acceptsMarkdown(req.Header.Get("Accept")) {
			markdown = true
		}
		if markdown {
			ctx := context.WithValue(req.Context(), markdownCtxKey{}, true)
			req = req.WithContext(ctx)
		}
		next.ServeHTTP(w, req)
	})
}

func acceptsMarkdown(accept string) bool {
	for value := range strings.SplitSeq(accept, ",") {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(value))
		if err != nil || !strings.EqualFold(mediaType, "text/markdown") {
			continue
		}
		quality := 1.0
		if rawQuality, ok := params["q"]; ok {
			quality, err = strconv.ParseFloat(rawQuality, 64)
			if err != nil {
				continue
			}
		}
		if quality > 0 {
			return true
		}
	}
	return false
}

func (h *PageHandler) servePageInput(req *http.Request) usecase.ServePageInput {
	return usecase.ServePageInput{
		Config:          h.config,
		DefaultHTMLLang: h.defaultHTMLLang,
		IsDev:           h.isDev,
		Manifest:        h.manifest,
		EntryName:       h.entryName,
		StaticPath:      h.staticPath,
		RequestPath:     req.URL.Path,
		Request:         req,
		Markdown:        req.Context().Value(markdownCtxKey{}) == true,
		Shell:           h.shell,
	}
}

func (h *PageHandler) dispatchPageOutput(w http.ResponseWriter, req *http.Request, output usecase.ServePageOutput) {
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
		if output.Markdown != "" {
			h.serveMarkdown(w, output.Markdown)
		} else {
			h.serveHTML(w, output.HTML)
		}

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
	if err := serveBifrostFile(w, req, h.assetsFS, rel, h.assetsFS != (embed.FS{}), "text/html; charset=utf-8"); err != nil {
		h.serveError(w, req, fmt.Errorf("failed to read %s file %s: %w", kind, rel, err))
	}
}

func (h *PageHandler) serveHTML(w http.ResponseWriter, htmlContent string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, htmlContent)
}

func (h *PageHandler) serveMarkdown(w http.ResponseWriter, mdContent string) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, mdContent)
}

func computeNextSteps(se *core.StructuredError) []string {
	switch se.ErrorType {
	case "Build Error":
		if se.Specifier != "" {
			return []string{
				fmt.Sprintf("Check that %q is a valid import.", se.Specifier),
				"Try running: bun install",
			}
		}
		if se.File != "" {
			return []string{fmt.Sprintf("Fix the error in %s:%d", se.File, se.Line)}
		}
		return []string{"Check the server logs for more details"}
	case "Import Error":
		return []string{"Verify the import path exists and the module is installed"}
	case "Render Error":
		return []string{"Check the component rendering logic in the stack trace"}
	default:
		return []string{"Check the server logs for more details"}
	}
}

func logRequestError(req *http.Request, err error) {
	slog.Error("request failed",
		"method", req.Method,
		"path", req.URL.Path,
		"error", err.Error(),
	)
}

func (h *PageHandler) serveError(w http.ResponseWriter, req *http.Request, err error) {
	var redirectErr core.RedirectError
	if errors.As(err, &redirectErr) {
		status := redirectErr.RedirectStatusCode()
		if status == 0 {
			status = http.StatusFound
		}
		http.Redirect(w, req, redirectErr.RedirectURL(), status)
		return
	}

	logRequestError(req, err)

	data := core.ErrorData{
		Message: err.Error(),
		IsDev:   h.isDev,
	}

	var se *core.StructuredError
	if errors.As(err, &se) && h.isDev {
		data.Structured = se
		if se.LineText != "" {
			data.CodeSnippet = se.LineText
		}
		data.NextSteps = computeNextSteps(se)
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
