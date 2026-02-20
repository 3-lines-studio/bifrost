package usecase

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/core"
)

//go:embed client_entry_template.txt
var clientEntryTemplate string

type ServePageInput struct {
	Config      core.PageConfig
	IsDev       bool
	Mode        core.Mode
	Manifest    *core.Manifest
	EntryName   string
	StaticPath  string
	RequestPath string
	HasRenderer bool
	Request     *http.Request
}

type ServePageOutput struct {
	Action     core.PageAction
	HTML       string
	StaticPath string
	RoutePath  string
	Props      map[string]any
	NeedsSetup bool
	Error      error
}

type PageService struct {
	renderer Renderer
	fs       FileSystem
}

func NewPageService(renderer Renderer, fs FileSystem) *PageService {
	return &PageService{
		renderer: renderer,
		fs:       fs,
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
			buildErr := s.buildAndRender(ctx, input)
			if buildErr != nil {
				return ServePageOutput{
					Action: core.ActionNeedsSetup,
					Error:  buildErr,
				}
			}
			// After setup, render based on the page mode
			if input.Config.Mode == core.ModeStaticPrerender {
				return s.renderStaticPrerender(ctx, input)
			}
			return s.renderSSR(ctx, input)
		}
		return ServePageOutput{
			Action:     core.ActionNeedsSetup,
			NeedsSetup: true,
		}

	case core.ActionRenderClientOnlyShell:
		html, err := s.renderClientOnlyShell(input)
		return ServePageOutput{
			Action: core.ActionRenderClientOnlyShell,
			HTML:   html,
			Error:  err,
		}

	case core.ActionRenderStaticPrerender:
		return s.renderStaticPrerender(ctx, input)

	case core.ActionRenderSSR:
		return s.renderSSR(ctx, input)

	default:
		return ServePageOutput{
			Action: core.ActionRenderSSR,
			Error:  fmt.Errorf("unknown page action"),
		}
	}
}

func (s *PageService) renderClientOnlyShell(input ServePageInput) (string, error) {
	assets := core.GetAssets(input.Manifest, input.EntryName)

	return core.RenderHTMLShell(
		"",
		map[string]any{},
		assets.Script,
		"",
		assets.CSS,
		assets.Chunks,
	)
}

func (s *PageService) renderStaticPrerender(ctx context.Context, input ServePageInput) ServePageOutput {
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

		if s.renderer == nil {
			return ServePageOutput{
				Action: core.ActionRenderStaticPrerender,
				Error:  fmt.Errorf("renderer not available for static prerender"),
			}
		}

		page, err := s.renderer.Render(input.Config.ComponentPath, props)
		if err != nil {
			return ServePageOutput{
				Action: core.ActionRenderStaticPrerender,
				Error:  err,
			}
		}

		html, err := s.renderPageHTML(input, props, page)
		return ServePageOutput{
			Action: core.ActionRenderStaticPrerender,
			HTML:   html,
			Props:  props,
			Error:  err,
		}
	}

	if s.renderer == nil {
		return ServePageOutput{
			Action: core.ActionRenderStaticPrerender,
			Error:  fmt.Errorf("renderer not available"),
		}
	}

	page, err := s.renderer.Render(input.Config.ComponentPath, map[string]any{})
	if err != nil {
		return ServePageOutput{
			Action: core.ActionRenderStaticPrerender,
			Error:  err,
		}
	}

	html, err := s.renderPageHTML(input, map[string]any{}, page)
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

	if s.renderer == nil {
		return ServePageOutput{
			Action: core.ActionRenderSSR,
			Error:  fmt.Errorf("renderer not available for SSR"),
		}
	}

	renderPath := input.Config.ComponentPath
	if !input.IsDev {
		renderPath = core.ResolveRenderPath(input.IsDev, input.StaticPath, input.Config.ComponentPath)
	}

	page, err := s.renderer.Render(renderPath, props)
	if err != nil {
		return ServePageOutput{
			Action: core.ActionRenderSSR,
			Error:  err,
		}
	}

	html, err := s.renderPageHTML(input, props, page)
	return ServePageOutput{
		Action: core.ActionRenderSSR,
		HTML:   html,
		Props:  props,
		Error:  err,
	}
}

func (s *PageService) renderPageHTML(input ServePageInput, props map[string]any, page core.RenderedPage) (string, error) {
	assets := core.GetAssets(input.Manifest, input.EntryName)

	return core.RenderHTMLShell(
		page.Body,
		props,
		assets.Script,
		page.Head,
		assets.CSS,
		assets.Chunks,
	)
}

func (s *PageService) buildAndRender(ctx context.Context, input ServePageInput) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	outdir := filepath.Join(cwd, ".bifrost/dist")
	entryDir := filepath.Join(cwd, ".bifrost/entries")

	if err := os.MkdirAll(entryDir, 0755); err != nil {
		return fmt.Errorf("failed to create entries directory: %w", err)
	}

	entryFile := filepath.Join(entryDir, input.EntryName+".tsx")

	componentPath := input.Config.ComponentPath
	// Make path relative to project root from the entries directory
	if !strings.HasPrefix(componentPath, "./") && !strings.HasPrefix(componentPath, "/") {
		componentPath = "./" + componentPath
	}
	// Entry is in .bifrost/entries/, need to go up two levels to reach project root
	if strings.HasPrefix(componentPath, "./") {
		componentPath = "../../" + componentPath[2:]
	}

	entryContent := strings.ReplaceAll(clientEntryTemplate, "COMPONENT_PATH", componentPath)

	if err := os.WriteFile(entryFile, []byte(entryContent), 0644); err != nil {
		return fmt.Errorf("failed to write entry file: %w", err)
	}

	entrypoints := []string{entryFile}
	entryNames := []string{input.EntryName}

	return s.renderer.Build(entrypoints, outdir, entryNames)
}
