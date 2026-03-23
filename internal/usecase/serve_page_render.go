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

func (s *PageService) renderClientOnlyShell(state pageRequestState) (string, error) {
	input := state.input
	shell, err := s.resolveShell(state)
	if err != nil {
		return "", err
	}

	if input.IsDev && s.renderer != nil {
		ssrPath := filepath.Join(".bifrost/ssr", input.EntryName+"-ssr.js")
		if _, err := os.Stat(ssrPath); err == nil {
			page, err := s.renderer.Render(ssrPath, map[string]any{})
			if err == nil {
				lang, htmlClass, _ := core.ResolveHTMLDocumentAttrs(input.DefaultHTMLLang, input.Config.HTMLLang, input.Config.HTMLClass, nil)
				return shell.Render(page.Body, nil, page.Head, lang, htmlClass)
			}
		}
	}

	lang, htmlClass, _ := core.ResolveHTMLDocumentAttrs(input.DefaultHTMLLang, input.Config.HTMLLang, input.Config.HTMLClass, nil)
	return shell.Render("", nil, "", lang, htmlClass)
}

func (s *PageService) renderStaticPrerender(ctx context.Context, state pageRequestState) ServePageOutput {
	input := state.input
	requestPath := core.NormalizePath(input.RequestPath)

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

		page, err := s.renderer.Render(state.renderPath, propsForReact)
		if err != nil {
			return ServePageOutput{
				Action: core.ActionRenderStaticPrerender,
				Error:  err,
			}
		}

		html, err := s.renderPageHTMLWithArtifacts(state, propsForReact, page, lang, htmlClass)
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

	lang, htmlClass, propsForReact := core.ResolveHTMLDocumentAttrs(input.DefaultHTMLLang, input.Config.HTMLLang, input.Config.HTMLClass, nil)

	page, err := s.renderer.Render(state.renderPath, propsForReact)
	if err != nil {
		return ServePageOutput{
			Action: core.ActionRenderStaticPrerender,
			Error:  err,
		}
	}

	html, err := s.renderPageHTMLWithArtifacts(state, propsForReact, page, lang, htmlClass)
	return ServePageOutput{
		Action: core.ActionRenderStaticPrerender,
		HTML:   html,
		Error:  err,
	}
}

func (s *PageService) renderSSR(ctx context.Context, state pageRequestState) ServePageOutput {
	input := state.input
	var props map[string]any
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
	propsJSON, err := core.MarshalBifrostPropsJSON(propsForReact)
	if err != nil {
		return ServePageOutput{
			Action: core.ActionRenderSSR,
			Error:  err,
		}
	}
	shell, err := s.resolveShell(state)
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
		err := s.renderer.RenderBodyStream(rCtx, state.renderPath, propsForReact, w, doFlush,
			func(head string) error {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				if err := shell.WritePreamble(w, head, lang, htmlClass); err != nil {
					return err
				}
				doFlush()
				return nil
			})
		if err != nil {
			return err
		}
		if err := shell.WriteSuffix(w, propsJSON); err != nil {
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
	return s.renderPageHTMLWithArtifacts(s.prepareRequest(input), props, page, htmlLang, htmlClass)
}

func (s *PageService) renderPageHTMLWithArtifacts(state pageRequestState, props map[string]any, page core.RenderedPage, htmlLang string, htmlClass string) (string, error) {
	shell, err := s.resolveShell(state)
	if err != nil {
		return "", err
	}
	return shell.Render(page.Body, props, page.Head, htmlLang, htmlClass)
}

func (s *PageService) resolveShell(state pageRequestState) (core.HTMLDocumentShell, error) {
	if state.shell != nil {
		return *state.shell, nil
	}
	return core.NewHTMLDocumentShell(
		state.artifacts.Script,
		state.artifacts.CriticalCSS,
		core.StylesheetHrefsFor(state.artifacts),
		state.artifacts.Chunks,
	)
}
