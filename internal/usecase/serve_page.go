package usecase

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/3-lines-studio/bifrost/internal/adapters/framework"
	"github.com/3-lines-studio/bifrost/internal/core"
)

type ServePageInput struct {
	Config          core.PageConfig
	DefaultHTMLLang string
	IsDev           bool
	Manifest        *core.Manifest
	EntryName       string
	StaticPath      string
	RequestPath     string
	HasRenderer     bool
	Request         *http.Request
}

type ServePageOutput struct {
	Action     core.PageAction
	HTML       string
	StaticPath string
	RoutePath  string
	Props      map[string]any
	NeedsSetup bool
	Error      error
	// Stream is set for SSR when the HTML response should be written with chunked flushing (see PageHandler).
	Stream func(http.ResponseWriter) error
}

type PageService struct {
	renderer   Renderer
	fs         FileSystem
	adapter    core.FrameworkAdapter
	buildGroup singleflightGroup
}

type pageRequestState struct {
	input      ServePageInput
	entry      *core.ManifestEntry
	decision   core.PageDecision
	artifacts  core.PageArtifacts
	renderPath string
}

func NewPageService(renderer Renderer, fs FileSystem, adapter core.FrameworkAdapter) *PageService {
	if adapter == nil {
		adapter = framework.DefaultAdapter()
	}
	return &PageService{
		renderer: renderer,
		fs:       fs,
		adapter:  adapter,
	}
}

func (s *PageService) ServePage(ctx context.Context, input ServePageInput) ServePageOutput {
	return s.executeRequest(ctx, s.prepareRequest(input))
}

func (s *PageService) prepareRequest(input ServePageInput) pageRequestState {
	var entry *core.ManifestEntry
	if input.Manifest != nil {
		if e, ok := input.Manifest.Entries[input.EntryName]; ok {
			entry = &e
		}
	}

	req := core.PageRequest{
		IsDev:       input.IsDev,
		Mode:        input.Config.Mode,
		RequestPath: input.RequestPath,
		HasManifest: input.Manifest != nil,
		EntryName:   input.EntryName,
		StaticPath:  input.StaticPath,
		HasRenderer: s.renderer != nil,
	}

	return pageRequestState{
		input:      input,
		entry:      entry,
		decision:   core.DecidePageAction(req, entry),
		artifacts:  core.ResolvePageArtifacts(input.Manifest, input.EntryName),
		renderPath: s.resolveRenderPath(input),
	}
}

func (s *PageService) executeRequest(ctx context.Context, state pageRequestState) ServePageOutput {
	switch state.decision.Action {
	case core.ActionServeStaticFile:
		return ServePageOutput{
			Action:     core.ActionServeStaticFile,
			StaticPath: state.decision.StaticPath,
		}

	case core.ActionServeRouteFile:
		return ServePageOutput{
			Action:    core.ActionServeRouteFile,
			RoutePath: state.decision.HTMLPath,
		}

	case core.ActionNotFound:
		return ServePageOutput{
			Action: core.ActionNotFound,
		}

	case core.ActionNeedsSetup:
		if state.input.IsDev && s.renderer != nil {
			buildErr := s.buildGroup.Do(state.input.EntryName, func() error {
				return s.buildAndRender(ctx, state.input)
			})
			if buildErr != nil {
				return ServePageOutput{
					Action: core.ActionNeedsSetup,
					Error:  buildErr,
				}
			}
			return s.renderForMode(ctx, s.prepareRequest(state.input))
		}
		return ServePageOutput{
			Action:     core.ActionNeedsSetup,
			NeedsSetup: true,
		}

	case core.ActionRenderClientOnlyShell,
		core.ActionRenderStaticPrerender,
		core.ActionRenderSSR:
		return s.renderForMode(ctx, state)

	default:
		return ServePageOutput{
			Action: core.ActionRenderSSR,
			Error:  fmt.Errorf("unknown page action"),
		}
	}
}

func (s *PageService) renderForMode(ctx context.Context, state pageRequestState) ServePageOutput {
	switch state.input.Config.Mode {
	case core.ModeClientOnly:
		html, err := s.renderClientOnlyShell(state)
		return ServePageOutput{
			Action: core.ActionRenderClientOnlyShell,
			HTML:   html,
			Error:  err,
		}
	case core.ModeStaticPrerender:
		return s.renderStaticPrerender(ctx, state)
	default:
		return s.renderSSR(ctx, state)
	}
}

func (s *PageService) buildAndRender(ctx context.Context, input ServePageInput) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	return CompileDevPageOnDemand(s.renderer, cwd, input.EntryName, input.Config, s.adapter)
}
