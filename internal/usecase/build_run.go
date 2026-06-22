package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/3-lines-studio/bifrost/internal/adapters/cli"
	"github.com/3-lines-studio/bifrost/internal/adapters/framework"
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
	framework        core.Framework
	adapter          core.FrameworkAdapter
}

func (p buildPage) entryPath(entriesDir string) string {
	return filepath.Join(entriesDir, p.entryName+p.adapter.EntryFileExtension())
}

func (p buildPage) ssrEntryName() string {
	return p.entryName + "-ssr"
}

func (p buildPage) ssrEntryPath(entriesDir string) string {
	return filepath.Join(entriesDir, p.ssrEntryName()+p.adapter.EntryFileExtension())
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
		bifrostDir:    filepath.Join(input.AppRoot, ".bifrost"),
		outdir:        filepath.Join(input.AppRoot, ".bifrost", "dist"),
		ssrDir:        filepath.Join(input.AppRoot, ".bifrost", "ssr"),
		entriesDir:    filepath.Join(input.AppRoot, ".bifrost", "entries"),
		pagesDir:      filepath.Join(input.AppRoot, ".bifrost", "pages"),
		runtimeDir:    filepath.Join(input.AppRoot, ".bifrost", "runtime"),
		publicDir:     filepath.Join(input.AppRoot, "public"),
		publicDestDir: filepath.Join(input.AppRoot, ".bifrost", "public"),
		manifestPath:  filepath.Join(input.AppRoot, ".bifrost", "manifest.json"),
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
		fw := core.FrameworkFromComponentPath(config.ComponentPath)
		page := buildPage{
			config:           config,
			entryName:        core.EntryNameForPath(config.ComponentPath),
			absComponentPath: filepath.Join(input.AppRoot, config.ComponentPath),
			modeLabel:        config.Mode.BuildLabel(),
			framework:        fw,
			adapter:          framework.ResolveAdapter(fw),
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

	type fwGroup struct {
		adapter core.FrameworkAdapter
		paths   []string
		names   []string
		pages   []buildPage
	}
	groups := map[string]*fwGroup{}

	for _, page := range run.pages {
		if page.config.Mode == core.ModeClientOnly {
			continue
		}

		ssrEntryPath := page.ssrEntryPath(run.paths.entriesDir)
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

		if err := WriteSSREntryFile(page.adapter, ssrEntryPath, importPath); err != nil {
			run.markSSRFailed(page.entryName)
			errors = append(errors, BuildError{
				Page:    page.config.ComponentPath,
				Message: "Failed to write SSR entry",
				Details: []string{err.Error()},
			})
			continue
		}

		fw := page.adapter.Name()
		grp, ok := groups[fw]
		if !ok {
			grp = &fwGroup{adapter: page.adapter}
			groups[fw] = grp
		}
		grp.paths = append(grp.paths, ssrEntryPath)
		grp.names = append(grp.names, page.entryName)
		grp.pages = append(grp.pages, page)
	}

	for fw, grp := range groups {
		if err := s.renderer.BuildSSR(grp.paths, run.paths.ssrDir, fw); err != nil {
			run.report.AddWarning("SSR build", fmt.Sprintf("Batch SSR build for %s failed; falling back to per-page builds", fw), []string{err.Error()})
			s.buildSSRBundlesIndividually(run, grp.pages, &errors)
		}
	}

	var allPages []buildPage
	for _, grp := range groups {
		allPages = append(allPages, grp.pages...)
	}
	s.validateSSRBundles(run, allPages, &errors)

	for _, grp := range groups {
		for _, entryName := range grp.names {
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
	}

	step.Success = len(errors) == 0
	run.report.EndStep(step, step.Success, "")
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
		ssrEntryPath := page.ssrEntryPath(run.paths.entriesDir)
		if err := s.renderer.BuildSSR([]string{ssrEntryPath}, run.paths.ssrDir, page.adapter.Name()); err != nil {
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
		entryPath := page.entryPath(run.paths.entriesDir)
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
			writeErr = WriteClientEntryFile(page.adapter, entryPath, importPath, core.ModeClientOnly)
		} else {
			writeErr = WriteClientEntryFile(page.adapter, entryPath, importPath, core.ModeSSR)
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

	type fwGroup struct {
		adapter core.FrameworkAdapter
		paths   []string
		names   []string
		pages   []buildPage
	}
	groups := map[string]*fwGroup{}

	for _, page := range run.pages {
		if run.ssrFailedFor(page.entryName) {
			continue
		}
		fw := page.adapter.Name()
		grp, ok := groups[fw]
		if !ok {
			grp = &fwGroup{adapter: page.adapter}
			groups[fw] = grp
		}
		grp.paths = append(grp.paths, page.entryPath(run.paths.entriesDir))
		grp.names = append(grp.names, page.entryName)
		grp.pages = append(grp.pages, page)
	}

	builtMap := make(map[string]core.ClientBuildResult)
	for fw, grp := range groups {
		if len(grp.paths) > 0 {
			var err error
			result, err := s.renderer.Build(grp.paths, run.paths.outdir, grp.names, fw)
			if err != nil {
				result = s.buildClientAssetsIndividually(run, grp.pages, grp.adapter, &errors)
			}
			maps.Copy(builtMap, result)
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

func (s *BuildService) buildClientAssetsIndividually(run *buildRun, pages []buildPage, adapter core.FrameworkAdapter, errors *[]BuildError) map[string]core.ClientBuildResult {
	builtMap := make(map[string]core.ClientBuildResult)
	for _, page := range pages {
		if run.ssrFailedFor(page.entryName) {
			continue
		}
		result, err := s.renderer.Build(
			[]string{page.entryPath(run.paths.entriesDir)},
			run.paths.outdir,
			[]string{page.entryName},
			adapter.Name(),
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

	seen := make(map[core.Framework]struct{})
	var frameworks []core.Framework
	for _, page := range run.pages {
		if _, ok := seen[page.framework]; !ok {
			seen[page.framework] = struct{}{}
			frameworks = append(frameworks, page.framework)
		}
	}

	step := run.report.StartStep("Compiling Bun runtime")
	if err := s.compileRuntimeFn(run.paths.bifrostDir, frameworks); err != nil {
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

	if err := s.runExportMode(run.input.ModuleRoot, run.input.AppRoot, run.paths.bifrostDir, run.manifest, run.input.MainFile); err != nil {
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
		_ = os.Remove(page.entryPath(run.paths.entriesDir))
		if page.config.Mode != core.ModeClientOnly {
			_ = os.Remove(page.ssrEntryPath(run.paths.entriesDir))
		}
	}
	run.report.EndStep(step, true, "")
}
