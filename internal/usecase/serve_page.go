package usecase

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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

func NewPageService(renderer Renderer, fs FileSystem, adapter core.FrameworkAdapter) *PageService {
	if adapter == nil {
		adapter = framework.NewReactAdapter()
	}
	return &PageService{
		renderer: renderer,
		fs:       fs,
		adapter:  adapter,
	}
}

func (s *PageService) serveRenderedForPageMode(ctx context.Context, input ServePageInput) ServePageOutput {
	switch input.Config.Mode {
	case core.ModeClientOnly:
		html, err := s.renderClientOnlyShell(input)
		return ServePageOutput{
			Action: core.ActionRenderClientOnlyShell,
			HTML:   html,
			Error:  err,
		}
	case core.ModeStaticPrerender:
		return s.renderStaticPrerender(ctx, input)
	default:
		return s.renderSSR(ctx, input)
	}
}

func (s *PageService) ServePage(ctx context.Context, input ServePageInput) ServePageOutput {
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

	decision := core.DecidePageAction(req, entry)

	switch decision.Action {
	case core.ActionServeStaticFile:
		return ServePageOutput{
			Action:     core.ActionServeStaticFile,
			StaticPath: decision.StaticPath,
		}

	case core.ActionServeRouteFile:
		return ServePageOutput{
			Action:    core.ActionServeRouteFile,
			RoutePath: decision.HTMLPath,
		}

	case core.ActionNotFound:
		return ServePageOutput{
			Action: core.ActionNotFound,
		}

	case core.ActionNeedsSetup:
		if input.IsDev && s.renderer != nil {
			buildErr := s.buildGroup.Do(input.EntryName, func() error {
				return s.buildAndRender(ctx, input)
			})
			if buildErr != nil {
				return ServePageOutput{
					Action: core.ActionNeedsSetup,
					Error:  buildErr,
				}
			}
			return s.serveRenderedForPageMode(ctx, input)
		}
		return ServePageOutput{
			Action:     core.ActionNeedsSetup,
			NeedsSetup: true,
		}

	case core.ActionRenderClientOnlyShell,
		core.ActionRenderStaticPrerender,
		core.ActionRenderSSR:
		return s.serveRenderedForPageMode(ctx, input)

	default:
		return ServePageOutput{
			Action: core.ActionRenderSSR,
			Error:  fmt.Errorf("unknown page action"),
		}
	}
}

func (s *PageService) buildAndRender(ctx context.Context, input ServePageInput) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	outdir := filepath.Join(cwd, ".bifrost/dist")
	ssrDir := filepath.Join(cwd, ".bifrost/ssr")
	entryDir := filepath.Join(cwd, ".bifrost/entries")

	if err := os.MkdirAll(entryDir, 0755); err != nil {
		return fmt.Errorf("failed to create entries directory: %w", err)
	}

	componentPath := input.Config.ComponentPath

	if !strings.HasPrefix(componentPath, "./") && !strings.HasPrefix(componentPath, "/") {
		componentPath = "./" + componentPath
	}

	if strings.HasPrefix(componentPath, "./") {
		componentPath = "../../" + componentPath[2:]
	}

	entryFile := filepath.Join(entryDir, input.EntryName+s.adapter.EntryFileExtension())
	clientTemplate := s.adapter.ClientEntryTemplate(input.Config.Mode)
	clientContent := strings.ReplaceAll(clientTemplate, "COMPONENT_PATH", componentPath)

	if err := os.WriteFile(entryFile, []byte(clientContent), 0644); err != nil {
		return fmt.Errorf("failed to write client entry file: %w", err)
	}

	clientEntrypoints := []string{entryFile}
	clientEntryNames := []string{input.EntryName}

	if _, err := s.renderer.Build(clientEntrypoints, outdir, clientEntryNames); err != nil {
		return fmt.Errorf("failed to build client entry: %w", err)
	}

	shouldBuildSSR := input.IsDev ||
		input.Config.Mode == core.ModeSSR ||
		input.Config.Mode == core.ModeStaticPrerender

	if shouldBuildSSR {
		if err := os.MkdirAll(ssrDir, 0755); err != nil {
			return fmt.Errorf("failed to create SSR directory: %w", err)
		}

		ssrEntryName := input.EntryName + "-ssr"
		ssrEntryFile := filepath.Join(entryDir, ssrEntryName+s.adapter.EntryFileExtension())
		ssrTemplate := s.adapter.SSREntryTemplate()
		ssrContent := strings.ReplaceAll(ssrTemplate, "COMPONENT_PATH", componentPath)

		if err := os.WriteFile(ssrEntryFile, []byte(ssrContent), 0644); err != nil {
			return fmt.Errorf("failed to write SSR entry file: %w", err)
		}

		ssrEntrypoints := []string{ssrEntryFile}

		if err := s.renderer.BuildSSR(ssrEntrypoints, ssrDir); err != nil {
			return fmt.Errorf("failed to build SSR entry: %w", err)
		}
	}

	return nil
}
