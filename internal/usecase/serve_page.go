package usecase

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/3-lines-studio/bifrost/internal/adapters/framework"
	"github.com/3-lines-studio/bifrost/internal/core"
)

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

// singleflightGroup deduplicates concurrent dev builds for the same entry.
type singleflightGroup struct {
	mu sync.Mutex
	m  map[string]*singleflightCall
}

type singleflightCall struct {
	wg  sync.WaitGroup
	err error
}

func (g *singleflightGroup) Do(key string, fn func() error) error {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*singleflightCall)
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.err
	}
	c := &singleflightCall{}
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()

	return c.err
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
			// After setup, render based on the page mode
			if input.Config.Mode == core.ModeClientOnly {
				html, err := s.renderClientOnlyShell(input)
				return ServePageOutput{
					Action: core.ActionRenderClientOnlyShell,
					HTML:   html,
					Error:  err,
				}
			}
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

	// In dev mode, try to render with SSR for initial content
	if input.IsDev && s.renderer != nil {
		ssrPath := filepath.Join(".bifrost/ssr", input.EntryName+"-ssr.js")
		if _, err := os.Stat(ssrPath); err == nil {
			page, err := s.renderer.Render(ssrPath, map[string]any{})
			if err == nil {
				return core.RenderHTMLShell(
					page.Body,
					map[string]any{},
					assets.Script,
					page.Head,
					assets.CSS,
					assets.Chunks,
				)
			}
			// If SSR render fails, fall through to empty shell
		}
	}

	// Production or fallback: empty shell (will be hydrated on client)
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

		if s.renderer == nil {
			return ServePageOutput{
				Action: core.ActionRenderStaticPrerender,
				Error:  fmt.Errorf("renderer not available for static prerender"),
			}
		}

		page, err := s.renderer.Render(renderPath, props)
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

	page, err := s.renderer.Render(renderPath, map[string]any{})
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

	renderPath := s.resolveRenderPath(input)

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
	ssrDir := filepath.Join(cwd, ".bifrost/ssr")
	entryDir := filepath.Join(cwd, ".bifrost/entries")

	if err := os.MkdirAll(entryDir, 0755); err != nil {
		return fmt.Errorf("failed to create entries directory: %w", err)
	}

	componentPath := input.Config.ComponentPath
	// Make path relative to project root from the entries directory
	if !strings.HasPrefix(componentPath, "./") && !strings.HasPrefix(componentPath, "/") {
		componentPath = "./" + componentPath
	}
	// Entry is in .bifrost/entries/, need to go up two levels to reach project root
	if strings.HasPrefix(componentPath, "./") {
		componentPath = "../../" + componentPath[2:]
	}

	// Build client entry
	entryFile := filepath.Join(entryDir, input.EntryName+s.adapter.EntryFileExtension())
	clientTemplate := s.adapter.ClientEntryTemplate(input.Config.Mode)
	clientContent := strings.ReplaceAll(clientTemplate, "COMPONENT_PATH", componentPath)

	if err := os.WriteFile(entryFile, []byte(clientContent), 0644); err != nil {
		return fmt.Errorf("failed to write client entry file: %w", err)
	}

	clientEntrypoints := []string{entryFile}
	clientEntryNames := []string{input.EntryName}

	if err := s.renderer.Build(clientEntrypoints, outdir, clientEntryNames); err != nil {
		return fmt.Errorf("failed to build client entry: %w", err)
	}

	// Build SSR entry in dev mode for all page types (for initial render),
	// or in production for SSR and StaticPrerender modes
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
		// Build SSR with target=bun
		if err := s.renderer.BuildSSR(ssrEntrypoints, ssrDir); err != nil {
			return fmt.Errorf("failed to build SSR entry: %w", err)
		}
	}

	return nil
}
