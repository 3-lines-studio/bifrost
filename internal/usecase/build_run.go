package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/adapters/cli"
	"github.com/3-lines-studio/bifrost/internal/adapters/react"
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

func (p buildPage) entryPath(entriesDir string) string {
	return filepath.Join(entriesDir, p.entryName+react.EntryFileExtension)
}

func (p buildPage) ssrEntryName() string {
	return p.entryName + "-ssr"
}

func (p buildPage) ssrEntryPath(entriesDir string) string {
	return filepath.Join(entriesDir, p.ssrEntryName()+react.EntryFileExtension)
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
		publicDir:     resolvePublicDir(input.AppRoot, input.ModuleRoot),
		publicDestDir: filepath.Join(input.AppRoot, ".bifrost", "public"),
		manifestPath:  filepath.Join(input.AppRoot, ".bifrost", "manifest.json"),
	}

	runtimeName := "bun"
	if strings.EqualFold(os.Getenv("BIFROST_JS_RUNTIME"), "sobek") {
		runtimeName = "sobek"
	}
	run := &buildRun{
		input:           input,
		paths:           paths,
		report:          cli.NewBuildReport(s.cli, paths.bifrostDir),
		pages:           make([]buildPage, len(pageConfigs)),
		manifest:        &core.Manifest{Runtime: runtimeName, Entries: make(map[string]core.ManifestEntry, len(pageConfigs))},
		defaultHTMLLang: defaultHTMLLang,
		ssrFailed:       make(map[string]struct{}),
	}
	run.report.SetPageCount(len(pageConfigs))

	entryComponents := make(map[string]string, len(pageConfigs))
	for i, config := range pageConfigs {
		entryName := core.EntryNameForPath(config.ComponentPath)
		if previous, exists := entryComponents[entryName]; exists && previous != config.ComponentPath {
			return nil, fmt.Errorf(
				"components %q and %q map to the same build entry %q",
				previous,
				config.ComponentPath,
				entryName,
			)
		}
		entryComponents[entryName] = config.ComponentPath

		page := buildPage{
			config:           config,
			entryName:        entryName,
			absComponentPath: resolveComponentPath(input.AppRoot, input.ModuleRoot, config.ComponentPath),
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

func (s *BuildService) copyPublicAssets(run *buildRun) error {
	if err := s.copyPublicDir(run.paths.publicDir, run.paths.publicDestDir); err != nil {
		return fmt.Errorf("failed to copy public assets: %w", err)
	}
	return nil
}

func (s *BuildService) buildSSRBundles(run *buildRun) {
	step := run.report.StartStep("Building SSR bundles")
	errors := make([]BuildError, 0)
	var paths []string
	var names []string
	var pages []buildPage

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

		if err := WriteSSREntryFile(ssrEntryPath, importPath); err != nil {
			run.markSSRFailed(page.entryName)
			errors = append(errors, BuildError{
				Page:    page.config.ComponentPath,
				Message: "Failed to write SSR entry",
				Details: []string{err.Error()},
			})
			continue
		}

		paths = append(paths, ssrEntryPath)
		names = append(names, page.entryName)
		pages = append(pages, page)
	}

	if len(paths) > 0 {
		if err := s.renderer.BuildSSR(paths, run.paths.ssrDir); err != nil {
			run.report.AddWarning("SSR build", "Batch SSR build failed; falling back to per-page builds", []string{err.Error()})
			s.buildSSRBundlesIndividually(run, pages, &errors)
		}
	}

	s.validateSSRBundles(run, pages, &errors)

	for _, entryName := range names {
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

		if writeErr := WriteClientEntryFile(entryPath, importPath, page.config.Mode); writeErr != nil {
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
	var paths []string
	var names []string
	var pages []buildPage

	for _, page := range run.pages {
		if run.ssrFailedFor(page.entryName) {
			continue
		}
		paths = append(paths, page.entryPath(run.paths.entriesDir))
		names = append(names, page.entryName)
		pages = append(pages, page)
	}

	builtMap := make(map[string]core.ClientBuildResult)
	if len(paths) > 0 {
		result, err := s.renderer.Build(paths, run.paths.outdir, names)
		if err != nil {
			result = s.buildClientAssetsIndividually(run, pages, &errors)
		}
		maps.Copy(builtMap, result)
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

func (s *BuildService) buildClientAssetsIndividually(run *buildRun, pages []buildPage, errors *[]BuildError) map[string]core.ClientBuildResult {
	builtMap := make(map[string]core.ClientBuildResult)
	for _, page := range pages {
		if run.ssrFailedFor(page.entryName) {
			continue
		}
		result, err := s.renderer.Build(
			[]string{page.entryPath(run.paths.entriesDir)},
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
			"",
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
	if strings.EqualFold(os.Getenv("BIFROST_JS_RUNTIME"), "sobek") {
		return os.RemoveAll(run.paths.runtimeDir)
	}

	step := run.report.StartStep("Compiling Bun runtime")
	if err := s.compileRuntimeFn(run.paths.bifrostDir); err != nil {
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
	if err := removeStaticSSRBundles(run); err != nil {
		run.report.AddError("StaticPrerender", "Failed to remove build-only SSR bundles", []string{err.Error()})
		run.report.EndStep(step, false, "")
		return err
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

func removeStaticSSRBundles(run *buildRun) error {
	for _, page := range run.pages {
		if page.config.Mode != core.ModeStaticPrerender {
			continue
		}
		entry := run.manifest.Entries[page.entryName]
		if entry.SSR != "" {
			rel := strings.TrimPrefix(filepath.ToSlash(entry.SSR), "/")
			if !strings.HasPrefix(rel, "ssr/") {
				return fmt.Errorf("static entry %q has invalid SSR path %q", page.entryName, entry.SSR)
			}
			if err := os.Remove(filepath.Join(run.paths.bifrostDir, filepath.FromSlash(rel))); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove static SSR bundle %q: %w", entry.SSR, err)
			}
		}
		matches, err := filepath.Glob(filepath.Join(run.paths.ssrDir, page.entryName+"-ssr.*"))
		if err != nil {
			return fmt.Errorf("find static SSR artifacts for %q: %w", page.entryName, err)
		}
		for _, match := range matches {
			if err := os.Remove(match); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove static SSR artifact %q: %w", match, err)
			}
		}
		entry.SSR = ""
		run.manifest.Entries[page.entryName] = entry
	}
	if !run.needsRuntime {
		if err := os.RemoveAll(run.paths.ssrDir); err != nil {
			return fmt.Errorf("remove static SSR directory: %w", err)
		}
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
	_ = os.Remove(run.paths.entriesDir)
	_ = os.Remove(run.paths.ssrDir)
	run.report.EndStep(step, true, "")
}

func resolveComponentPath(appRoot, moduleRoot, componentPath string) string {
	resolved := filepath.Join(appRoot, componentPath)
	if _, err := os.Stat(resolved); err == nil {
		return resolved
	}
	return filepath.Join(moduleRoot, componentPath)
}

func resolvePublicDir(appRoot, moduleRoot string) string {
	appCandidate := filepath.Join(appRoot, "public")
	if info, err := os.Stat(appCandidate); err == nil && info.IsDir() {
		return appCandidate
	}
	return filepath.Join(moduleRoot, "public")
}
