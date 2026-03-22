package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/3-lines-studio/bifrost/internal/adapters/cli"
	"github.com/3-lines-studio/bifrost/internal/adapters/framework"
	"github.com/3-lines-studio/bifrost/internal/core"
)

type BuildInput struct {
	MainFile    string
	OriginalCwd string
}

type BuildOutput struct {
	Success bool
	Error   error
}

type BuildError struct {
	Page    string
	Message string
	Details []string
}

type BuildService struct {
	renderer Renderer
	fs       FileSystem
	cli      CLIOutput
	adapter  core.FrameworkAdapter
}

func NewBuildService(renderer Renderer, fs FileSystem, cli CLIOutput, adapter core.FrameworkAdapter) *BuildService {
	if adapter == nil {
		adapter = framework.NewReactAdapter()
	}
	return &BuildService{
		renderer: renderer,
		fs:       fs,
		cli:      cli,
		adapter:  adapter,
	}
}

type pageMetadata struct {
	config           core.PageConfig
	entryName        string
	absComponentPath string
	modeStr          string
}

func (s *BuildService) BuildProject(ctx context.Context, input BuildInput) BuildOutput {
	s.cli.PrintHeader("Bifrost Build")

	pageConfigs, fileDefaultHTMLLang, err := s.scanPages(input.MainFile)
	if err != nil {
		return BuildOutput{
			Success: false,
			Error:   fmt.Errorf("failed to scan pages: %w", err),
		}
	}

	if len(pageConfigs) == 0 {
		return BuildOutput{
			Success: false,
			Error:   fmt.Errorf("no pages found"),
		}
	}

	pages := make([]pageMetadata, len(pageConfigs))
	hasStaticPrerender := false
	needsRuntime := false
	for i, config := range pageConfigs {
		entryName := core.EntryNameForPath(config.ComponentPath)
		var modeStr string
		switch config.Mode {
		case core.ModeClientOnly:
			modeStr = "client"
		case core.ModeStaticPrerender:
			modeStr = "static"
			hasStaticPrerender = true
		default:
			modeStr = "ssr"
		}
		if config.Mode == core.ModeSSR {
			needsRuntime = true
		}
		pages[i] = pageMetadata{
			config:           config,
			entryName:        entryName,
			absComponentPath: filepath.Join(input.OriginalCwd, config.ComponentPath),
			modeStr:          modeStr,
		}
	}

	report := cli.NewBuildReport(s.cli, filepath.Join(input.OriginalCwd, ".bifrost"))
	report.SetPageCount(len(pageConfigs))

	bifrostDir := filepath.Join(input.OriginalCwd, ".bifrost")
	outdir := filepath.Join(bifrostDir, "dist")
	ssrDir := filepath.Join(bifrostDir, "ssr")
	entriesDir := filepath.Join(bifrostDir, "entries")
	pagesDir := filepath.Join(bifrostDir, "pages")

	stepDirs := report.StartStep("Creating output directories")
	if err := os.MkdirAll(outdir, 0755); err != nil {
		return BuildOutput{Success: false, Error: fmt.Errorf("failed to create dist dir: %w", err)}
	}
	if err := os.MkdirAll(ssrDir, 0755); err != nil {
		return BuildOutput{Success: false, Error: fmt.Errorf("failed to create ssr dir: %w", err)}
	}
	if err := os.MkdirAll(entriesDir, 0755); err != nil {
		return BuildOutput{Success: false, Error: fmt.Errorf("failed to create entries dir: %w", err)}
	}
	if err := os.MkdirAll(pagesDir, 0755); err != nil {
		return BuildOutput{Success: false, Error: fmt.Errorf("failed to create pages dir: %w", err)}
	}
	report.EndStep(stepDirs, true, "")

	publicDir := filepath.Join(input.OriginalCwd, "public")
	publicDestDir := filepath.Join(bifrostDir, "public")
	if err := s.copyPublicDir(publicDir, publicDestDir); err != nil {
		report.AddWarning("Public assets", "Failed to copy public assets", []string{err.Error()})
	}

	manifest := &core.Manifest{
		Entries: make(map[string]core.ManifestEntry, len(pages)),
	}

	stepSSR := report.StartStep("Building SSR bundles")
	ssrErrors := make([]BuildError, 0)
	ssrFailed := make(map[string]struct{})

	for i := range pages {
		pm := &pages[i]
		if pm.config.Mode == core.ModeClientOnly {
			continue
		}

		ssrEntryName := pm.entryName + "-ssr"
		ssrEntryPath := filepath.Join(entriesDir, ssrEntryName+s.adapter.EntryFileExtension())

		importPath, err := s.calculateImportPath(ssrEntryPath, pm.absComponentPath)
		if err != nil {
			ssrFailed[pm.entryName] = struct{}{}
			ssrErrors = append(ssrErrors, BuildError{
				Page:    pm.config.ComponentPath,
				Message: "Failed to calculate import path",
				Details: []string{err.Error()},
			})
			continue
		}

		if err := s.writeSSREntry(ssrEntryPath, importPath); err != nil {
			ssrFailed[pm.entryName] = struct{}{}
			ssrErrors = append(ssrErrors, BuildError{
				Page:    pm.config.ComponentPath,
				Message: "Failed to write SSR entry",
				Details: []string{err.Error()},
			})
			continue
		}

		if err := s.renderer.BuildSSR([]string{ssrEntryPath}, ssrDir); err != nil {
			ssrFailed[pm.entryName] = struct{}{}
			ssrErrors = append(ssrErrors, parseBuildError(pm.entryName, err))
			continue
		}

		manifest.Entries[pm.entryName] = core.ManifestEntry{
			Script: "/dist/" + pm.entryName + ".js",
			CSS:    "/dist/" + pm.entryName + ".css",
			SSR:    "/ssr/" + ssrEntryName + ".js",
			Mode:   "ssr",
		}
	}

	report.EndStep(stepSSR, len(ssrErrors) == 0, "")
	for _, err := range ssrErrors {
		if err.Page != "" {
			report.AddError(err.Page, err.Message, err.Details)
		} else {
			report.AddWarning("SSR build", err.Message, err.Details)
		}
	}

	stepClient := report.StartStep("Generating client entry files")
	clientEntryErrors := make([]BuildError, 0)

	for i := range pages {
		pm := &pages[i]
		entryPath := filepath.Join(entriesDir, pm.entryName+s.adapter.EntryFileExtension())

		importPath, err := s.calculateImportPath(entryPath, pm.absComponentPath)
		if err != nil {
			clientEntryErrors = append(clientEntryErrors, BuildError{
				Page:    pm.config.ComponentPath,
				Message: "Failed to calculate import path",
				Details: []string{err.Error()},
			})
			continue
		}

		if pm.config.Mode == core.ModeClientOnly {
			if err := s.writeClientOnlyEntry(entryPath, importPath); err != nil {
				clientEntryErrors = append(clientEntryErrors, BuildError{
					Page:    pm.entryName,
					Message: "Failed to write client-only entry",
					Details: []string{err.Error()},
				})
				continue
			}
		} else {
			if err := s.writeHydrationEntry(entryPath, importPath); err != nil {
				clientEntryErrors = append(clientEntryErrors, BuildError{
					Page:    pm.entryName,
					Message: "Failed to write hydration entry",
					Details: []string{err.Error()},
				})
				continue
			}
		}
	}
	report.EndStep(stepClient, len(clientEntryErrors) == 0, "")
	for _, err := range clientEntryErrors {
		report.AddWarning(err.Page, err.Message, err.Details)
	}

	stepClientAssets := report.StartStep("Building client assets")
	clientAssetErrors := make([]BuildError, 0)

	entryPaths := make([]string, 0, len(pages))
	entryNames := make([]string, 0, len(pages))
	for i := range pages {
		pm := &pages[i]
		if _, skip := ssrFailed[pm.entryName]; skip {
			continue
		}
		entryPaths = append(entryPaths, filepath.Join(entriesDir, pm.entryName+s.adapter.EntryFileExtension()))
		entryNames = append(entryNames, pm.entryName)
	}

	if len(entryPaths) > 0 {
		builtMap, batchErr := s.renderer.Build(entryPaths, outdir, entryNames)
		if batchErr != nil {

			builtMap = make(map[string]core.ClientBuildResult)
			for i := range pages {
				pm := &pages[i]
				ep := filepath.Join(entriesDir, pm.entryName+s.adapter.EntryFileExtension())
				one, err := s.renderer.Build([]string{ep}, outdir, []string{pm.entryName})
				if err != nil {
					clientAssetErrors = append(clientAssetErrors, parseBuildError(pm.entryName, err))
					continue
				}
				builtMap[pm.entryName] = one[pm.entryName]
			}
		}
		for i := range pages {
			pm := &pages[i]
			built, ok := builtMap[pm.entryName]
			if !ok {
				continue
			}
			entry := manifest.Entries[pm.entryName]
			entry.Script = built.Script
			entry.CriticalCSS = built.CriticalCSS
			entry.CSS = built.CSS
			entry.CSSFiles = built.CSSFiles
			entry.Chunks = built.Chunks
			entry.Mode = pm.modeStr
			manifest.Entries[pm.entryName] = entry
		}
	}
	report.EndStep(stepClientAssets, len(clientAssetErrors) == 0, "")
	for _, err := range clientAssetErrors {
		report.AddError(err.Page, err.Message, err.Details)
	}

	s.populateCriticalCSS(ctx, bifrostDir, pages, manifest)

	stepHTML := report.StartStep("Generating ClientOnly HTML shells")
	htmlErrors := make([]BuildError, 0)
	for i := range pages {
		pm := &pages[i]
		if pm.config.Mode != core.ModeClientOnly {
			continue
		}

		title := s.extractTitleFromComponent(pm.absComponentPath)

		mentry := manifest.Entries[pm.entryName]
		htmlPath := filepath.Join(pagesDir, pm.entryName+".html")
		lang := pm.config.HTMLLang
		if lang == "" {
			lang = fileDefaultHTMLLang
		}
		lang = core.SanitizeHTMLLang(lang)
		if err := s.writeClientOnlyHTML(htmlPath, title, mentry.Script, mentry.CriticalCSS, core.StylesheetHrefs(mentry.CSS, mentry.CSSFiles), mentry.Chunks, lang, pm.config.HTMLClass); err != nil {
			htmlErrors = append(htmlErrors, BuildError{
				Page:    pm.entryName,
				Message: "Failed to generate HTML shell",
				Details: []string{err.Error()},
			})
			continue
		}

		entry := manifest.Entries[pm.entryName]
		entry.HTML = "/pages/" + pm.entryName + ".html"
		manifest.Entries[pm.entryName] = entry
	}
	report.EndStep(stepHTML, len(htmlErrors) == 0, "")
	for _, err := range htmlErrors {
		report.AddWarning(err.Page, err.Message, err.Details)
	}

	manifestPath := filepath.Join(bifrostDir, "manifest.json")
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return BuildOutput{Success: false, Error: fmt.Errorf("failed to marshal manifest: %w", err)}
	}
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return BuildOutput{Success: false, Error: fmt.Errorf("failed to write manifest: %w", err)}
	}

	shouldCompileRuntime := needsRuntime || hasStaticPrerender

	if shouldCompileRuntime {
		stepRuntime := report.StartStep("Compiling Bun runtime")
		if err := s.compileEmbeddedRuntime(bifrostDir); err != nil {
			report.AddError("Runtime", "Failed to compile embedded runtime", []string{err.Error()})
			report.EndStep(stepRuntime, false, "")
			return BuildOutput{Success: false, Error: fmt.Errorf("runtime compilation failed: %w", err)}
		}
		report.EndStep(stepRuntime, true, "")
	}

	stepExport := report.StartStep("Building StaticPrerender pages")
	if hasStaticPrerender {
		exportErr := s.runExportMode(input.OriginalCwd, bifrostDir, manifest, input.MainFile)
		if exportErr != nil {
			report.AddError("StaticPrerender", "Export mode failed", []string{exportErr.Error()})
			report.EndStep(stepExport, false, "")
			return BuildOutput{Success: false, Error: fmt.Errorf("export mode failed: %w", exportErr)}
		}
		report.EndStep(stepExport, true, "")

		if !needsRuntime {
			runtimeDir := filepath.Join(bifrostDir, "runtime")
			if err := os.RemoveAll(runtimeDir); err != nil {
				report.AddWarning("Cleanup", "Failed to remove runtime directory", []string{err.Error()})
			}
		}

		manifestData, err = json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return BuildOutput{Success: false, Error: fmt.Errorf("failed to marshal manifest after export: %w", err)}
		}
		if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
			return BuildOutput{Success: false, Error: fmt.Errorf("failed to write manifest after export: %w", err)}
		}
	} else {
		report.EndStep(stepExport, true, "")
	}

	stepCleanup := report.StartStep("Cleaning up entry files")
	for i := range pages {
		pm := &pages[i]
		ext := s.adapter.EntryFileExtension()
		_ = os.Remove(filepath.Join(entriesDir, pm.entryName+ext))
		if pm.config.Mode != core.ModeClientOnly {
			_ = os.Remove(filepath.Join(entriesDir, pm.entryName+"-ssr"+ext))
		}
	}
	report.EndStep(stepCleanup, true, "")

	report.Render()

	return BuildOutput{Success: !report.HasFailures()}
}
