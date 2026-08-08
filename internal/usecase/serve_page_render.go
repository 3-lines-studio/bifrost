package usecase

import (
	"context"
	"fmt"
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

		var props any
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

		page, err := renderWithContext(ctx, s.renderer, state.renderPath, propsForReact)
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

	page, err := renderWithContext(ctx, s.renderer, state.renderPath, propsForReact)
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

	var syncProps any
	var propsMs float64
	if input.Config.PropsLoader != nil {
		var err error
		propsStart := time.Now()
		syncProps, err = input.Config.PropsLoader(input.Request)
		propsMs = float64(time.Since(propsStart)) / float64(time.Millisecond)
		if err != nil {
			return ServePageOutput{
				Action:  core.ActionRenderSSR,
				Error:   err,
				PropsMs: propsMs,
			}
		}
	}

	pre := core.PreLoaderResult{}
	if input.Pre != nil {
		pre = *input.Pre
	} else if input.Config.PreLoader != nil {
		preResult, err := input.Config.PreLoader(input.Request)
		if err != nil {
			return ServePageOutput{Action: core.ActionRenderSSR, Error: err}
		}
		pre = preResult
	}

	lang, htmlClass, syncPropsForReact := core.ResolveHTMLDocumentAttrsWithPre(input.DefaultHTMLLang, input.Config.HTMLLang, input.Config.HTMLClass, pre, syncProps)

	if s.renderer == nil {
		return ServePageOutput{
			Action: core.ActionRenderSSR,
			Error:  fmt.Errorf("renderer not available for SSR"),
		}
	}

	renderStart := time.Now()
	page, err := renderWithContext(ctx, s.renderer, state.renderPath, syncPropsForReact)
	renderMs := float64(time.Since(renderStart)) / float64(time.Millisecond)
	if err != nil {
		return ServePageOutput{
			Action: core.ActionRenderSSR,
			Error:  err,
		}
	}

	if input.Markdown {
		assembleStart := time.Now()
		md, err := convertHTMLToMarkdown(page.Body)
		assembleMs := float64(time.Since(assembleStart)) / float64(time.Millisecond)
		if err != nil {
			return ServePageOutput{
				Action:   core.ActionRenderSSR,
				Error:    err,
				RenderMs: renderMs,
				PropsMs:  propsMs,
			}
		}
		return ServePageOutput{
			Action:     core.ActionRenderSSR,
			Markdown:   md,
			IsMarkdown: true,
			Props:      syncPropsForReact,
			RenderMs:   renderMs,
			PropsMs:    propsMs,
			AssembleMs: assembleMs,
		}
	}

	assembleStart := time.Now()
	html, err := s.renderPageHTMLWithArtifacts(state, syncPropsForReact, page, lang, htmlClass)
	assembleMs := float64(time.Since(assembleStart)) / float64(time.Millisecond)
	return ServePageOutput{
		Action:     core.ActionRenderSSR,
		HTML:       html,
		Props:      syncPropsForReact,
		RenderMs:   renderMs,
		PropsMs:    propsMs,
		AssembleMs: assembleMs,
		Page:       page,
		Error:      err,
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

func (s *PageService) renderPageHTML(input ServePageInput, props any, page core.RenderedPage, htmlLang string, htmlClass string) (string, error) {
	return s.renderPageHTMLWithArtifacts(s.prepareRequest(input), props, page, htmlLang, htmlClass)
}

func (s *PageService) renderPageHTMLWithArtifacts(state pageRequestState, props any, page core.RenderedPage, htmlLang string, htmlClass string) (string, error) {
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
