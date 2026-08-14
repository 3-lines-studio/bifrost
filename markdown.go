package bifrost

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

type markdownKey struct{}

func markdownRequested(ctx context.Context) bool {
	requested, _ := ctx.Value(markdownKey{}).(bool)
	return requested
}

func (a *App) ResolveMarkdown(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		_, pattern := mux.Handler(request)
		_, serverRoute := a.runtime.serverPatterns[pattern]
		if acceptsMarkdown(request.Header.Get("Accept")) && serverRoute {
			mux.ServeHTTP(w, withMarkdown(request))
			return
		}

		urlPath := request.URL.Path
		escapedPath := request.URL.EscapedPath()
		if len(urlPath) <= 3 || len(escapedPath) <= 3 || !strings.EqualFold(urlPath[len(urlPath)-3:], ".md") || !strings.EqualFold(escapedPath[len(escapedPath)-3:], ".md") {
			mux.ServeHTTP(w, request)
			return
		}
		if pattern != "" && !serverRoute && !subtreePattern(pattern) {
			mux.ServeHTTP(w, request)
			return
		}

		rewritten := request.Clone(request.Context())
		rewritten.URL.Path = urlPath[:len(urlPath)-3]
		if request.URL.RawPath != "" {
			rewritten.URL.RawPath = escapedPath[:len(escapedPath)-3]
		}
		rewritten.RequestURI = rewritten.URL.RequestURI()
		_, pattern = mux.Handler(rewritten)
		if _, exists := a.runtime.serverPatterns[pattern]; !exists {
			mux.ServeHTTP(w, request)
			return
		}
		mux.ServeHTTP(w, withMarkdown(rewritten))
	})
}

func subtreePattern(pattern string) bool {
	if index := strings.LastIndexByte(pattern, ' '); index >= 0 {
		pattern = pattern[index+1:]
	}
	if index := strings.IndexByte(pattern, '/'); index >= 0 {
		pattern = pattern[index:]
	}
	return strings.HasSuffix(pattern, "/") || strings.HasSuffix(pattern, "...}")
}

func withMarkdown(request *http.Request) *http.Request {
	return request.WithContext(context.WithValue(request.Context(), markdownKey{}, true))
}

func acceptsMarkdown(header string) bool {
	markdownQuality := -1.0
	htmlQuality := -1.0
	textQuality := -1.0
	anyQuality := -1.0
	for value := range strings.SplitSeq(header, ",") {
		mediaType, params, err := mime.ParseMediaType(value)
		if err != nil {
			continue
		}
		quality := 1.0
		if rawQuality, exists := params["q"]; exists {
			quality, err = strconv.ParseFloat(rawQuality, 64)
			if err != nil || quality < 0 || quality > 1 {
				continue
			}
		}
		switch mediaType {
		case "text/markdown":
			markdownQuality = max(markdownQuality, quality)
		case "text/html":
			htmlQuality = max(htmlQuality, quality)
		case "text/*":
			textQuality = max(textQuality, quality)
		case "*/*":
			anyQuality = max(anyQuality, quality)
		}
	}
	if markdownQuality <= 0 {
		return false
	}
	if htmlQuality < 0 {
		htmlQuality = textQuality
	}
	if htmlQuality < 0 {
		htmlQuality = anyQuality
	}
	return markdownQuality >= htmlQuality
}

type markdownRenderSink struct {
	writer  http.ResponseWriter
	body    bytes.Buffer
	hasHead bool
	started bool
	limits  Limits
}

func (s *markdownRenderSink) Head(head []byte) error {
	if len(head) > s.limits.MaxHeadBytes {
		return fmt.Errorf("renderer head exceeds %d bytes", s.limits.MaxHeadBytes)
	}
	if s.hasHead {
		return errors.New("bifrost: renderer emitted head more than once")
	}
	s.hasHead = true
	return nil
}

func (s *markdownRenderSink) Body(body []byte) error {
	if len(body) > s.limits.MaxFrameBytes {
		return fmt.Errorf("renderer frame exceeds %d bytes", s.limits.MaxFrameBytes)
	}
	if !s.hasHead {
		return errors.New("bifrost: renderer emitted body before head")
	}
	if s.started {
		return errors.New("bifrost: renderer emitted body after completion")
	}
	if len(body) > s.limits.MaxMarkdownBytes-s.body.Len() {
		return fmt.Errorf("markdown body exceeds %d bytes", s.limits.MaxMarkdownBytes)
	}
	_, err := s.body.Write(body)
	return err
}

func (s *markdownRenderSink) committed() bool { return s.started }

func (s *markdownRenderSink) finish() error {
	if !s.hasHead {
		return errors.New("bifrost: renderer did not emit head")
	}
	if s.started {
		return errors.New("bifrost: renderer completed more than once")
	}
	markdown, err := htmltomarkdown.ConvertString(s.body.String())
	if err != nil {
		return fmt.Errorf("bifrost: convert page to markdown: %w", err)
	}
	s.started = true
	s.writer.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	s.writer.Header().Set("Cache-Control", "no-store")
	s.writer.WriteHeader(http.StatusOK)
	_, err = io.WriteString(s.writer, markdown)
	return err
}
