package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/3-lines-studio/bifrost/internal/adapters/cli"
	"github.com/3-lines-studio/bifrost/internal/core"
)

type buildPaths struct {
	bifrostDir    string
	outdir        string
	ssrDir        string
	entriesDir    string
	pagesDir      string
	runtimeDir    string
	publicDir     string
	publicDestDir string
	manifestPath  string
}

type buildPage struct {
	config           core.PageConfig
	entryName        string
	absComponentPath string
	modeLabel        string
}

func (p buildPage) entryPath(adapter core.FrameworkAdapter, entriesDir string) string {
	return filepath.Join(entriesDir, p.entryName+adapter.EntryFileExtension())
}

func (p buildPage) ssrEntryName() string {
	return p.entryName + "-ssr"
}

func (p buildPage) ssrEntryPath(adapter core.FrameworkAdapter, entriesDir string) string {
	return filepath.Join(entriesDir, p.ssrEntryName()+adapter.EntryFileExtension())
}

type buildRun struct {
	input              BuildInput
	paths              buildPaths
	report             *cli.BuildReport
	pages              []buildPage
	manifest           *core.Manifest
	defaultHTMLLang    string
	hasStaticPrerender bool
	needsRuntime       bool
	ssrFailed          map[string]struct{}
}

func (r *buildRun) updateManifestEntry(entryName string, update func(*core.ManifestEntry)) {
	entry := r.manifest.Entries[entryName]
	update(&entry)
	r.manifest.Entries[entryName] = entry
}

func (r *buildRun) markSSRFailed(entryName string) {
	r.ssrFailed[entryName] = struct{}{}
}

func (r *buildRun) ssrFailedFor(entryName string) bool {
	_, ok := r.ssrFailed[entryName]
	return ok
}

func (s *BuildService) newBuildRun(input BuildInput) (*buildRun, error) {
	pageConfigs, defaultHTMLLang, err := s.scanPages(input.MainFile)
	if err != nil {
		return nil, fmt.Errorf("failed to scan pages: %w", err)
	}
	if len(pageConfigs) == 0 {
		return nil, fmt.Errorf("no pages found")
	}

	paths := buildPaths{
		bifrostDir:    filepath.Join(input.OriginalCwd, ".bifrost"),
		outdir:        filepath.Join(input.OriginalCwd, ".bifrost", "dist"),
		ssrDir:        filepath.Join(input.OriginalCwd, ".bifrost", "ssr"),
		entriesDir:    filepath.Join(input.OriginalCwd, ".bifrost", "entries"),
		pagesDir:      filepath.Join(input.OriginalCwd, ".bifrost", "pages"),
		runtimeDir:    filepath.Join(input.OriginalCwd, ".bifrost", "runtime"),
		publicDir:     filepath.Join(input.OriginalCwd, "public"),
		publicDestDir: filepath.Join(input.OriginalCwd, ".bifrost", "public"),
		manifestPath:  filepath.Join(input.OriginalCwd, ".bifrost", "manifest.json"),
	}

	run := &buildRun{
		input:           input,
		paths:           paths,
		report:          cli.NewBuildReport(s.cli, paths.bifrostDir),
		pages:           make([]buildPage, len(pageConfigs)),
		manifest:        &core.Manifest{Entries: make(map[string]core.ManifestEntry, len(pageConfigs))},
		defaultHTMLLang: defaultHTMLLang,
		ssrFailed:       make(map[string]struct{}),
	}
	run.report.SetPageCount(len(pageConfigs))

	for i, config := range pageConfigs {
		page := buildPage{
			config:           config,
			entryName:        core.EntryNameForPath(config.ComponentPath),
			absComponentPath: filepath.Join(input.OriginalCwd, config.ComponentPath),
			modeLabel:        config.Mode.BuildLabel(),
		}
		run.pages[i] = page
		if config.Mode == core.ModeStaticPrerender {
			run.hasStaticPrerender = true
		}
		if config.Mode.NeedsSSRBundle() {
			run.needsRuntime = true
		}
	}

	return run, nil
}

func (s *BuildService) createOutputDirs(run *buildRun) error {
	step := run.report.StartStep("Creating output directories")

	cleanPaths := []struct {
		path string
		name string
	}{
		{path: run.paths.outdir, name: "dist"},
		{path: run.paths.ssrDir, name: "ssr"},
		{path: run.paths.entriesDir, name: "entries"},
		{path: run.paths.pagesDir, name: "pages"},
		{path: run.paths.runtimeDir, name: "runtime"},
		{path: run.paths.publicDestDir, name: "public"},
	}

	for _, dir := range cleanPaths {
		if err := os.RemoveAll(dir.path); err != nil {
			run.report.EndStep(step, false, fmt.Sprintf("failed to clean %s dir: %v", dir.name, err))
			return fmt.Errorf("failed to clean %s dir: %w", dir.name, err)
		}
	}

	dirs := []struct {
		path string
		name string
	}{
		{path: run.paths.outdir, name: "dist"},
		{path: run.paths.ssrDir, name: "ssr"},
		{path: run.paths.entriesDir, name: "entries"},
		{path: run.paths.pagesDir, name: "pages"},
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir.path, 0o755); err != nil {
			run.report.EndStep(step, false, fmt.Sprintf("failed to create %s dir: %v", dir.name, err))
			return fmt.Errorf("failed to create %s dir: %w", dir.name, err)
		}
	}

	run.report.EndStep(step, true, "")
	return nil
}

func (s *BuildService) copyPublicAssets(run *buildRun) {
	if err := s.copyPublicDir(run.paths.publicDir, run.paths.publicDestDir); err != nil {
		run.report.AddWarning("Public assets", "Failed to copy public assets", []string{err.Error()})
	}
}

func (s *BuildService) buildSSRBundles(run *buildRun) {
	step := run.report.StartStep("Building SSR bundles")
	errors := make([]BuildError, 0)
	var batchFallbackWarning []string

	entryPaths := make([]string, 0, len(run.pages))
	entryNames := make([]string, 0, len(run.pages))
	pagesToBuild := make([]buildPage, 0, len(run.pages))

	for _, page := range run.pages {
		if page.config.Mode == core.ModeClientOnly {
			continue
		}

		ssrEntryPath := page.ssrEntryPath(s.adapter, run.paths.entriesDir)
		importPath, err := CalculateImportPath(ssrEntryPath, page.absComponentPath)
		if err != nil {
			run.markSSRFailed(page.entryName)
			errors = append(errors, BuildError{
				Page:    page.config.ComponentPath,
				Message: "Failed to calculate import path",
				Details: []string{err.Error()},
			})
			continue
		}

		if err := s.writeSSREntry(ssrEntryPath, importPath); err != nil {
			run.markSSRFailed(page.entryName)
			errors = append(errors, BuildError{
				Page:    page.config.ComponentPath,
				Message: "Failed to write SSR entry",
				Details: []string{err.Error()},
			})
			continue
		}

		entryPaths = append(entryPaths, ssrEntryPath)
		entryNames = append(entryNames, page.entryName)
		pagesToBuild = append(pagesToBuild, page)
	}

	if len(entryPaths) > 0 {
		if err := s.renderer.BuildSSR(entryPaths, run.paths.ssrDir); err != nil {
			batchFallbackWarning = []string{err.Error()}
			s.buildSSRBundlesIndividually(run, pagesToBuild, &errors)
		}
	}

	s.validateSSRBundles(run, pagesToBuild, &errors)

	for _, entryName := range entryNames {
		if run.ssrFailedFor(entryName) {
			continue
		}
		run.updateManifestEntry(entryName, func(entry *core.ManifestEntry) {
			entry.Script = "/dist/" + entryName + ".js"
			entry.CSS = "/dist/" + entryName + ".css"
			entry.SSR = "/ssr/" + entryName + "-ssr.js"
			entry.Mode = "ssr"
		})
	}

	step.Success = len(errors) == 0
	run.report.EndStep(step, step.Success, "")
	if len(batchFallbackWarning) > 0 {
		run.report.AddWarning("SSR build", "Batch SSR build failed; fell back to per-page builds", batchFallbackWarning)
	}
	for _, err := range errors {
		if err.Page != "" {
			run.report.AddError(err.Page, err.Message, err.Details)
		} else {
			run.report.AddWarning("SSR build", err.Message, err.Details)
		}
	}
}

func (s *BuildService) buildSSRBundlesIndividually(run *buildRun, pages []buildPage, errors *[]BuildError) {
	for _, page := range pages {
		ssrEntryPath := page.ssrEntryPath(s.adapter, run.paths.entriesDir)
		if err := s.renderer.BuildSSR([]string{ssrEntryPath}, run.paths.ssrDir); err != nil {
			run.markSSRFailed(page.entryName)
			*errors = append(*errors, parseBuildError(page.entryName, err))
		}
	}
}

func (s *BuildService) validateSSRBundles(run *buildRun, pages []buildPage, errors *[]BuildError) {
	for _, page := range pages {
		if run.ssrFailedFor(page.entryName) {
			continue
		}
		if _, err := normalizeSSRBundle(run.paths.ssrDir, page.entryName); err != nil {
			run.markSSRFailed(page.entryName)
			*errors = append(*errors, BuildError{
				Page:    page.config.ComponentPath,
				Message: "SSR bundle missing after build",
				Details: []string{err.Error()},
			})
		}
	}
}

func (s *BuildService) generateClientEntries(run *buildRun) {
	step := run.report.StartStep("Generating client entry files")
	errors := make([]BuildError, 0)

	for _, page := range run.pages {
		entryPath := page.entryPath(s.adapter, run.paths.entriesDir)
		importPath, err := CalculateImportPath(entryPath, page.absComponentPath)
		if err != nil {
			errors = append(errors, BuildError{
				Page:    page.config.ComponentPath,
				Message: "Failed to calculate import path",
				Details: []string{err.Error()},
			})
			continue
		}

		var writeErr error
		if page.config.Mode == core.ModeClientOnly {
			writeErr = s.writeClientOnlyEntry(entryPath, importPath)
		} else {
			writeErr = s.writeHydrationEntry(entryPath, importPath)
		}
		if writeErr != nil {
			errors = append(errors, BuildError{
				Page:    page.entryName,
				Message: "Failed to write client entry",
				Details: []string{writeErr.Error()},
			})
		}
	}

	step.Success = len(errors) == 0
	run.report.EndStep(step, step.Success, "")
	for _, err := range errors {
		run.report.AddWarning(err.Page, err.Message, err.Details)
	}
}

func (s *BuildService) buildClientAssets(run *buildRun) {
	step := run.report.StartStep("Building client assets")
	errors := make([]BuildError, 0)

	entryPaths := make([]string, 0, len(run.pages))
	entryNames := make([]string, 0, len(run.pages))
	for _, page := range run.pages {
		if run.ssrFailedFor(page.entryName) {
			continue
		}
		entryPaths = append(entryPaths, page.entryPath(s.adapter, run.paths.entriesDir))
		entryNames = append(entryNames, page.entryName)
	}

	builtMap := make(map[string]core.ClientBuildResult)
	if len(entryPaths) > 0 {
		var err error
		builtMap, err = s.renderer.Build(entryPaths, run.paths.outdir, entryNames)
		if err != nil {
			builtMap = s.buildClientAssetsIndividually(run, &errors)
		}
	}

	for _, page := range run.pages {
		built, ok := builtMap[page.entryName]
		if !ok {
			continue
		}
		run.updateManifestEntry(page.entryName, func(entry *core.ManifestEntry) {
			entry.Script = built.Script
			entry.CriticalCSS = built.CriticalCSS
			entry.CSS = built.CSS
			entry.CSSFiles = built.CSSFiles
			entry.Chunks = built.Chunks
			entry.Mode = page.modeLabel
		})
	}

	step.Success = len(errors) == 0
	run.report.EndStep(step, step.Success, "")
	for _, err := range errors {
		run.report.AddError(err.Page, err.Message, err.Details)
	}
}

func (s *BuildService) buildClientAssetsIndividually(run *buildRun, errors *[]BuildError) map[string]core.ClientBuildResult {
	builtMap := make(map[string]core.ClientBuildResult)
	for _, page := range run.pages {
		if run.ssrFailedFor(page.entryName) {
			continue
		}
		result, err := s.renderer.Build(
			[]string{page.entryPath(s.adapter, run.paths.entriesDir)},
			run.paths.outdir,
			[]string{page.entryName},
		)
		if err != nil {
			*errors = append(*errors, parseBuildError(page.entryName, err))
			continue
		}
		builtMap[page.entryName] = result[page.entryName]
	}
	return builtMap
}

func (s *BuildService) generateClientOnlyHTML(run *buildRun) {
	step := run.report.StartStep("Generating ClientOnly HTML shells")
	errors := make([]BuildError, 0)

	for _, page := range run.pages {
		if page.config.Mode != core.ModeClientOnly {
			continue
		}

		entry := run.manifest.Entries[page.entryName]
		htmlPath := filepath.Join(run.paths.pagesDir, page.entryName+".html")
		lang := page.config.HTMLLang
		if lang == "" {
			lang = run.defaultHTMLLang
		}
		lang = core.SanitizeHTMLLang(lang)

		err := s.writeClientOnlyHTML(
			htmlPath,
			s.extractTitleFromComponent(page.absComponentPath),
			entry.Script,
			entry.CriticalCSS,
			core.StylesheetHrefs(entry.CSS, entry.CSSFiles),
			entry.Chunks,
			lang,
			page.config.HTMLClass,
		)
		if err != nil {
			errors = append(errors, BuildError{
				Page:    page.entryName,
				Message: "Failed to generate HTML shell",
				Details: []string{err.Error()},
			})
			continue
		}

		run.updateManifestEntry(page.entryName, func(entry *core.ManifestEntry) {
			entry.HTML = "/pages/" + page.entryName + ".html"
		})
	}

	step.Success = len(errors) == 0
	run.report.EndStep(step, step.Success, "")
	for _, err := range errors {
		run.report.AddWarning(err.Page, err.Message, err.Details)
	}
}

func (s *BuildService) writeManifest(run *buildRun) error {
	manifestData, err := json.MarshalIndent(run.manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}
	if err := os.WriteFile(run.paths.manifestPath, manifestData, 0o644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}
	return nil
}

func (s *BuildService) compileRuntime(run *buildRun) error {
	if !run.needsRuntime && !run.hasStaticPrerender {
		return nil
	}

	step := run.report.StartStep("Compiling Bun runtime")
	if err := s.compileEmbeddedRuntime(run.paths.bifrostDir); err != nil {
		run.report.AddError("Runtime", "Failed to compile embedded runtime", []string{err.Error()})
		run.report.EndStep(step, false, "")
		return fmt.Errorf("runtime compilation failed: %w", err)
	}
	run.report.EndStep(step, true, "")
	return nil
}

func (s *BuildService) exportStaticPrerender(_ context.Context, run *buildRun) error {
	step := run.report.StartStep("Building StaticPrerender pages")
	if !run.hasStaticPrerender {
		run.report.EndStep(step, true, "")
		return nil
	}

	if err := s.runExportMode(run.input.OriginalCwd, run.paths.bifrostDir, run.manifest, run.input.MainFile); err != nil {
		run.report.AddError("StaticPrerender", "Export mode failed", []string{err.Error()})
		run.report.EndStep(step, false, "")
		return fmt.Errorf("export mode failed: %w", err)
	}
	run.report.EndStep(step, true, "")

	if !run.needsRuntime {
		if err := os.RemoveAll(run.paths.runtimeDir); err != nil {
			run.report.AddWarning("Cleanup", "Failed to remove runtime directory", []string{err.Error()})
		}
	}

	manifestData, err := json.MarshalIndent(run.manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal manifest after export: %w", err)
	}
	if err := os.WriteFile(run.paths.manifestPath, manifestData, 0o644); err != nil {
		return fmt.Errorf("failed to write manifest after export: %w", err)
	}
	return nil
}

func (s *BuildService) cleanupEntryFiles(run *buildRun) {
	step := run.report.StartStep("Cleaning up entry files")
	for _, page := range run.pages {
		_ = os.Remove(page.entryPath(s.adapter, run.paths.entriesDir))
		if page.config.Mode != core.ModeClientOnly {
			_ = os.Remove(page.ssrEntryPath(s.adapter, run.paths.entriesDir))
		}
	}
	run.report.EndStep(step, true, "")
}
