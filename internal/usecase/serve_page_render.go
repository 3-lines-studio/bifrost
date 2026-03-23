package usecase

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func (s *PageService) renderClientOnlyShell(input ServePageInput) (string, error) {
	artifacts := core.ResolvePageArtifacts(input.Manifest, input.EntryName)

	if input.IsDev && s.renderer != nil {
		ssrPath := filepath.Join(".bifrost/ssr", input.EntryName+"-ssr.js")
		if _, err := os.Stat(ssrPath); err == nil {
			page, err := s.renderer.Render(ssrPath, map[string]any{})
			if err == nil {
				lang, htmlClass, _ := core.ResolveHTMLDocumentAttrs(input.DefaultHTMLLang, input.Config.HTMLLang, input.Config.HTMLClass, nil)
				return RenderHTMLDocumentFromPage(page, map[string]any{}, artifacts, lang, htmlClass)
			}
		}
	}

	lang, htmlClass, _ := core.ResolveHTMLDocumentAttrs(input.DefaultHTMLLang, input.Config.HTMLLang, input.Config.HTMLClass, nil)
	return core.RenderHTMLShell(
		"",
		map[string]any{},
		artifacts.Script,
		"",
		artifacts.CriticalCSS,
		core.StylesheetHrefsFor(artifacts),
		artifacts.Chunks,
		lang,
		htmlClass,
	)
}

func (s *PageService) renderStaticPrerender(ctx context.Context, input ServePageInput) ServePageOutput {
	requestPath := core.NormalizePath(input.RequestPath)

	renderPath := s.resolveRenderPath(input)

	if input.Config.StaticDataLoader != nil {
		entries, err := input.Config.StaticDataLoader(ctx)
		if err != nil {
			return ServePageOutput{
				Action: core.ActionRenderStaticPrerender,
				Error:  fmt.Errorf("failed to load static data: %w", err),
			}
		}

		var props map[string]any
		found := false
		for _, entry := range entries {
			if core.NormalizePath(entry.Path) == requestPath {
				props = entry.Props
				found = true
				break
			}
		}

		if !found {
			return ServePageOutput{
				Action: core.ActionNotFound,
			}
		}

		lang, htmlClass, propsForReact := core.ResolveHTMLDocumentAttrs(input.DefaultHTMLLang, input.Config.HTMLLang, input.Config.HTMLClass, props)

		if s.renderer == nil {
			return ServePageOutput{
				Action: core.ActionRenderStaticPrerender,
				Error:  fmt.Errorf("renderer not available for static prerender"),
			}
		}

		page, err := s.renderer.Render(renderPath, propsForReact)
		if err != nil {
			return ServePageOutput{
				Action: core.ActionRenderStaticPrerender,
				Error:  err,
			}
		}

		html, err := s.renderPageHTML(input, propsForReact, page, lang, htmlClass)
		return ServePageOutput{
			Action: core.ActionRenderStaticPrerender,
			HTML:   html,
			Props:  propsForReact,
			Error:  err,
		}
	}

	if s.renderer == nil {
		return ServePageOutput{
			Action: core.ActionRenderStaticPrerender,
			Error:  fmt.Errorf("renderer not available"),
		}
	}

	lang, htmlClass, propsForReact := core.ResolveHTMLDocumentAttrs(input.DefaultHTMLLang, input.Config.HTMLLang, input.Config.HTMLClass, map[string]any{})

	page, err := s.renderer.Render(renderPath, propsForReact)
	if err != nil {
		return ServePageOutput{
			Action: core.ActionRenderStaticPrerender,
			Error:  err,
		}
	}

	html, err := s.renderPageHTML(input, propsForReact, page, lang, htmlClass)
	return ServePageOutput{
		Action: core.ActionRenderStaticPrerender,
		HTML:   html,
		Error:  err,
	}
}

func (s *PageService) renderSSR(ctx context.Context, input ServePageInput) ServePageOutput {
	props := map[string]any{}
	if input.Config.PropsLoader != nil {
		var err error
		props, err = input.Config.PropsLoader(input.Request)
		if err != nil {
			return ServePageOutput{
				Action: core.ActionRenderSSR,
				Error:  err,
			}
		}
	}

	lang, htmlClass, propsForReact := core.ResolveHTMLDocumentAttrs(input.DefaultHTMLLang, input.Config.HTMLLang, input.Config.HTMLClass, props)

	if s.renderer == nil {
		return ServePageOutput{
			Action: core.ActionRenderSSR,
			Error:  fmt.Errorf("renderer not available for SSR"),
		}
	}

	renderPath := s.resolveRenderPath(input)

	artifacts := core.ResolvePageArtifacts(input.Manifest, input.EntryName)
	propsJSON, err := core.MarshalBifrostPropsJSON(propsForReact)
	if err != nil {
		return ServePageOutput{
			Action: core.ActionRenderSSR,
			Error:  err,
		}
	}

	flush := func(w http.ResponseWriter) func() {
		return func() {
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}

	streamFn := func(w http.ResponseWriter) error {
		doFlush := flush(w)
		rCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		err := s.renderer.RenderBodyStream(rCtx, renderPath, propsForReact, w, doFlush,
			func(head string) error {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				if err := WriteSSRHTMLPreamble(w, head, artifacts, lang, htmlClass); err != nil {
					return err
				}
				doFlush()
				return nil
			})
		if err != nil {
			return err
		}
		if err := core.WriteHTMLSuffix(w, propsJSON, artifacts.Script, artifacts.Chunks); err != nil {
			return err
		}
		doFlush()
		return nil
	}

	return ServePageOutput{
		Action: core.ActionRenderSSR,
		Stream: streamFn,
		Props:  propsForReact,
	}
}

func (s *PageService) resolveRenderPath(input ServePageInput) string {
	if !input.IsDev {
		return core.ResolveRenderPath(input.IsDev, input.StaticPath, input.Config.ComponentPath)
	}
	ssrPath := filepath.Join(".bifrost/ssr", input.EntryName+"-ssr.js")
	if _, err := os.Stat(ssrPath); err == nil {
		return ssrPath
	}
	return input.Config.ComponentPath
}

func (s *PageService) renderPageHTML(input ServePageInput, props map[string]any, page core.RenderedPage, htmlLang string, htmlClass string) (string, error) {
	artifacts := core.ResolvePageArtifacts(input.Manifest, input.EntryName)
	return RenderHTMLDocumentFromPage(page, props, artifacts, htmlLang, htmlClass)
}
