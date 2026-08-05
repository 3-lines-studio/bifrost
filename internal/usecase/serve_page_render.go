package usecase

import (
	"context"
	"fmt"
	"io"
	"log/slog"
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

		stream, err := s.streamRender(state, propsForReact, lang, htmlClass)
		return ServePageOutput{
			Action: core.ActionRenderStaticPrerender,
			Props:  propsForReact,
			Stream: stream,
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

	stream, err := s.streamRender(state, propsForReact, lang, htmlClass)
	return ServePageOutput{
		Action: core.ActionRenderStaticPrerender,
		Stream: stream,
		Error:  err,
	}
}

type pageTiming struct {
	propsDur    time.Duration
	renderStart time.Time
	renderDur   time.Duration
	entryName   string
	path        string
}

func (s *PageService) renderSSR(ctx context.Context, state pageRequestState) ServePageOutput {
	input := state.input
	var timing pageTiming
	timing.entryName = input.EntryName
	timing.path = input.RequestPath

	var syncProps any
	if input.Config.PropsLoader != nil {
		propsStart := time.Now()
		var err error
		syncProps, err = input.Config.PropsLoader(input.Request)
		timing.propsDur = time.Since(propsStart)
		if err != nil {
			return ServePageOutput{
				Action: core.ActionRenderSSR,
				Error:  err,
			}
		}
	}

	lang, htmlClass, syncPropsForReact := core.ResolveHTMLDocumentAttrs(input.DefaultHTMLLang, input.Config.HTMLLang, input.Config.HTMLClass, syncProps)

	if s.renderer == nil {
		return ServePageOutput{
			Action: core.ActionRenderSSR,
			Error:  fmt.Errorf("renderer not available for SSR"),
		}
	}

	if input.Markdown {
		timing.renderStart = time.Now()
		page, err := s.renderer.Render(state.renderPath, syncPropsForReact)
		timing.renderDur = time.Since(timing.renderStart)
		if err != nil {
			return ServePageOutput{
				Action: core.ActionRenderSSR,
				Error:  err,
			}
		}
		md, err := convertHTMLToMarkdown(page.Body)
		if err != nil {
			return ServePageOutput{
				Action: core.ActionRenderSSR,
				Error:  err,
			}
		}
		slog.Info("bifrost page timing",
			"entry", timing.entryName,
			"path", timing.path,
			"props_ms", timing.propsDur.Milliseconds(),
			"render_ms", timing.renderDur.Milliseconds(),
		)
		return ServePageOutput{
			Action:   core.ActionRenderSSR,
			Markdown: md,
			Props:    syncPropsForReact,
		}
	}

	shell, err := s.resolveShell(state)
	if err != nil {
		return ServePageOutput{
			Action: core.ActionRenderSSR,
			Error:  err,
		}
	}
	propsJSON, err := core.MarshalBifrostPropsJSON(syncPropsForReact)
	if err != nil {
		return ServePageOutput{
			Action: core.ActionRenderSSR,
			Error:  err,
		}
	}

	stream := func(w io.Writer) error {
		timing.renderStart = time.Now()
		err := s.renderer.RenderBodyTo(w, state.renderPath, syncPropsForReact, func(head string) error {
			return shell.WritePreamble(w, head, lang, htmlClass)
		})
		timing.renderDur = time.Since(timing.renderStart)
		if err != nil {
			return err
		}
		slog.Info("bifrost page timing",
			"entry", timing.entryName,
			"path", timing.path,
			"props_ms", timing.propsDur.Milliseconds(),
			"render_ms", timing.renderDur.Milliseconds(),
		)
		return shell.WriteSuffix(w, propsJSON)
	}

	return ServePageOutput{
		Action: core.ActionRenderSSR,
		Props:  syncPropsForReact,
		Stream: stream,
	}
}

// streamRender builds a closure that streams a rendered page to w: preamble
// with the rendered head, the body, then the props script and deferred scripts.
func (s *PageService) streamRender(state pageRequestState, propsForReact any, lang string, htmlClass string) (func(io.Writer) error, error) {
	shell, err := s.resolveShell(state)
	if err != nil {
		return nil, err
	}
	propsJSON, err := core.MarshalBifrostPropsJSON(propsForReact)
	if err != nil {
		return nil, err
	}
	return func(w io.Writer) error {
		err := s.renderer.RenderBodyTo(w, state.renderPath, propsForReact, func(head string) error {
			return shell.WritePreamble(w, head, lang, htmlClass)
		})
		if err != nil {
			return err
		}
		return shell.WriteSuffix(w, propsJSON)
	}, nil
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
