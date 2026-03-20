package usecase

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/adapters/cli"
	"github.com/3-lines-studio/bifrost/internal/adapters/framework"
	"github.com/3-lines-studio/bifrost/internal/core"
)

//go:embed clientonly_html_template.txt
var clientOnlyHTMLTemplate string

// Precompiled title extraction regexes (avoid per-call compilation).
var (
	titleRegex         = regexp.MustCompile(`<title>([^}]+?)</title>`)
	titleTemplateRegex = regexp.MustCompile(`<title>\{` + "`" + `([^}]+?)` + "`" + `\}</title>`)
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

// pageMetadata holds precomputed per-page data to avoid redundant calculations.
type pageMetadata struct {
	config           core.PageConfig
	entryName        string
	absComponentPath string
	modeStr          string
}

func (s *BuildService) BuildProject(ctx context.Context, input BuildInput) BuildOutput {
	s.cli.PrintHeader("Bifrost Build")

	pageConfigs, err := s.scanPages(input.MainFile)
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

	// Precompute per-page metadata once
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
			ssrErrors = append(ssrErrors, s.parseBuildError(pm.entryName, err))
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
			// One invalid entry fails the whole graph; retry per page so other routes still build.
			builtMap = make(map[string]core.ClientBuildResult)
			for i := range pages {
				pm := &pages[i]
				ep := filepath.Join(entriesDir, pm.entryName+s.adapter.EntryFileExtension())
				one, err := s.renderer.Build([]string{ep}, outdir, []string{pm.entryName})
				if err != nil {
					clientAssetErrors = append(clientAssetErrors, s.parseBuildError(pm.entryName, err))
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
			entry.CSS = built.CSS
			entry.Chunks = built.Chunks
			entry.Mode = pm.modeStr
			manifest.Entries[pm.entryName] = entry
		}
	}
	report.EndStep(stepClientAssets, len(clientAssetErrors) == 0, "")
	for _, err := range clientAssetErrors {
		report.AddError(err.Page, err.Message, err.Details)
	}

	// Generate ClientOnly HTML shells
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
		if err := s.writeClientOnlyHTML(htmlPath, title, mentry.Script, mentry.CSS, mentry.Chunks); err != nil {
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

	// Write manifest before export mode
	manifestPath := filepath.Join(bifrostDir, "manifest.json")
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return BuildOutput{Success: false, Error: fmt.Errorf("failed to marshal manifest: %w", err)}
	}
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return BuildOutput{Success: false, Error: fmt.Errorf("failed to write manifest: %w", err)}
	}

	// Compile runtime if needed for SSR pages (runtime) or static pages (export)
	// - SSR pages need runtime at production time
	// - Static pages need runtime at build time (for prerendering)
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

	// Only run export mode when static-prerender pages exist
	stepExport := report.StartStep("Building StaticPrerender pages")
	if hasStaticPrerender {
		exportErr := s.runExportMode(input.OriginalCwd, bifrostDir, manifest, input.MainFile)
		if exportErr != nil {
			report.AddError("StaticPrerender", "Export mode failed", []string{exportErr.Error()})
			report.EndStep(stepExport, false, "")
			return BuildOutput{Success: false, Error: fmt.Errorf("export mode failed: %w", exportErr)}
		}
		report.EndStep(stepExport, true, "")

		// For static-only apps, remove runtime after export to reduce binary size
		// Static apps don't need runtime at production time (pages are pre-rendered)
		if !needsRuntime {
			runtimeDir := filepath.Join(bifrostDir, "runtime")
			if err := os.RemoveAll(runtimeDir); err != nil {
				report.AddWarning("Cleanup", "Failed to remove runtime directory", []string{err.Error()})
			}
		}

		// Re-write manifest only when export ran (may have added static routes)
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

func (s *BuildService) scanPages(mainFile string) ([]core.PageConfig, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, mainFile, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var configs []core.PageConfig
	seen := make(map[string]bool)

	ast.Inspect(node, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		var funcName string
		argIndex := 1

		switch fn := callExpr.Fun.(type) {
		case *ast.SelectorExpr:
			funcName = fn.Sel.Name
		case *ast.Ident:
			funcName = fn.Name
		default:
			return true
		}

		if funcName != "Page" {
			return true
		}

		if len(callExpr.Args) <= argIndex {
			return true
		}

		firstArg := callExpr.Args[argIndex]
		lit, ok := firstArg.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			slog.Warn("Page call with non-string component path", "position", fset.Position(callExpr.Pos()))
			return true
		}

		path, err := strconv.Unquote(lit.Value)
		if err != nil {
			slog.Warn("Failed to unquote string", "position", fset.Position(lit.Pos()), "error", err)
			return true
		}

		mode, hasStaticDataLoader := s.detectPageMode(callExpr.Args[argIndex:])

		if !seen[path] {
			seen[path] = true
			configs = append(configs, core.PageConfig{
				ComponentPath:    path,
				Mode:             mode,
				StaticDataLoader: nil,
			})
			_ = hasStaticDataLoader
		}

		return true
	})

	return configs, nil
}

func (s *BuildService) detectPageMode(args []ast.Expr) (core.PageMode, bool) {
	hasClientOnly := false
	hasStaticPrerender := false
	hasStaticDataLoader := false

	for _, arg := range args {
		callExpr, ok := arg.(*ast.CallExpr)
		if !ok {
			continue
		}

		var funcName string
		switch fn := callExpr.Fun.(type) {
		case *ast.SelectorExpr:
			funcName = fn.Sel.Name
		case *ast.Ident:
			funcName = fn.Name
		}

		switch funcName {
		case "WithClient":
			hasClientOnly = true
		case "WithStatic":
			hasStaticPrerender = true
		case "WithStaticData":
			hasStaticPrerender = true
			hasStaticDataLoader = true
		}
	}

	if hasClientOnly && hasStaticPrerender {
		return core.ModeSSR, hasStaticDataLoader
	}

	if hasStaticPrerender {
		return core.ModeStaticPrerender, hasStaticDataLoader
	}

	if hasClientOnly {
		return core.ModeClientOnly, hasStaticDataLoader
	}

	return core.ModeSSR, hasStaticDataLoader
}

func (s *BuildService) calculateImportPath(entryPath, componentPath string) (string, error) {
	entryDir := filepath.Dir(entryPath)
	relPath, err := filepath.Rel(entryDir, componentPath)
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(relPath, ".") {
		relPath = "./" + relPath
	}

	return relPath, nil
}

func (s *BuildService) extractTitleFromComponent(componentPath string) string {
	data, err := os.ReadFile(componentPath)
	if err != nil {
		return ""
	}
	content := string(data)

	matches := titleRegex.FindStringSubmatch(content)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	matches = titleTemplateRegex.FindStringSubmatch(content)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	return ""
}

func (s *BuildService) writeClientOnlyHTML(htmlPath, title, script, css string, chunks []string) error {
	var chunkLines strings.Builder
	for _, c := range chunks {
		chunkLines.WriteString(`    <script src="`)
		chunkLines.WriteString(c)
		chunkLines.WriteString(`" type="module" defer></script>
`)
	}
	cssLink := ""
	if css != "" {
		cssLink = `    <link rel="stylesheet" href="` + css + `" media="print" onload="this.media='all'" />
    <noscript><link rel="stylesheet" href="` + css + `" /></noscript>
`
	}
	var modulePreload strings.Builder
	for _, c := range chunks {
		modulePreload.WriteString(`    <link rel="modulepreload" href="`)
		modulePreload.WriteString(c)
		modulePreload.WriteString(`" />
`)
	}
	modulePreload.WriteString(`    <link rel="modulepreload" href="`)
	modulePreload.WriteString(script)
	modulePreload.WriteString(`" />
`)
	html := clientOnlyHTMLTemplate
	html = strings.ReplaceAll(html, "TITLE_PLACEHOLDER", title)
	html = strings.ReplaceAll(html, "CSS_LINK_PLACEHOLDER", cssLink)
	html = strings.ReplaceAll(html, "MODULEPRELOAD_PLACEHOLDER", modulePreload.String())
	html = strings.ReplaceAll(html, "CHUNK_SCRIPTS_PLACEHOLDER", chunkLines.String())
	html = strings.ReplaceAll(html, "SCRIPT_SRC_PLACEHOLDER", script)
	return os.WriteFile(htmlPath, []byte(html), 0644)
}

func (s *BuildService) runExportMode(originalCwd, bifrostDir string, manifest *core.Manifest, mainFile string) error {
	binaryPath := filepath.Join(bifrostDir, "temp-app")
	cmd := exec.Command("go", "build", "-o", binaryPath, mainFile)
	cmd.Dir = originalCwd
	cmd.Env = append(os.Environ(),
		"BIFROST_EXPORT=1",
		"BIFROST_EXPORT_DIR="+bifrostDir,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to build app for export: %v\nOutput: %s", err, output)
	}

	if err := os.Chmod(binaryPath, 0755); err != nil {
		return fmt.Errorf("failed to make binary executable: %w", err)
	}

	defer func() { _ = os.Remove(binaryPath) }()

	exportCmd := exec.Command(binaryPath)
	exportCmd.Dir = originalCwd
	exportCmd.Env = append(os.Environ(),
		"BIFROST_EXPORT=1",
		"BIFROST_EXPORT_DIR="+bifrostDir,
	)
	exportCmd.Stdout = os.Stdout
	exportCmd.Stderr = os.Stderr

	if err := exportCmd.Run(); err != nil {
		return fmt.Errorf("export mode failed: %w", err)
	}

	exportManifestPath := filepath.Join(bifrostDir, "export-manifest.json")
	exportData, err := os.ReadFile(exportManifestPath)
	if err != nil {
		return fmt.Errorf("failed to read export manifest: %w", err)
	}

	var exportManifest core.Manifest
	if err := json.Unmarshal(exportData, &exportManifest); err != nil {
		return fmt.Errorf("failed to parse export manifest: %w", err)
	}

	for entryName, entry := range exportManifest.Entries {
		if existing, ok := manifest.Entries[entryName]; ok {
			existing.StaticRoutes = entry.StaticRoutes
			manifest.Entries[entryName] = existing
		} else {
			manifest.Entries[entryName] = entry
		}
	}

	_ = os.Remove(exportManifestPath)

	return nil
}

func (s *BuildService) writeSSREntry(entryPath, importPath string) error {
	content := strings.ReplaceAll(s.adapter.SSREntryTemplate(), "COMPONENT_PATH", importPath)
	return os.WriteFile(entryPath, []byte(content), 0644)
}

func (s *BuildService) writeClientOnlyEntry(entryPath, importPath string) error {
	content := strings.ReplaceAll(s.adapter.ClientEntryTemplate(core.ModeClientOnly), "COMPONENT_PATH", importPath)
	return os.WriteFile(entryPath, []byte(content), 0644)
}

func (s *BuildService) writeHydrationEntry(entryPath, importPath string) error {
	content := strings.ReplaceAll(s.adapter.ClientEntryTemplate(core.ModeSSR), "COMPONENT_PATH", importPath)
	return os.WriteFile(entryPath, []byte(content), 0644)
}

func (s *BuildService) compileEmbeddedRuntime(bifrostDir string) error {
	runtimeDir := filepath.Join(bifrostDir, "runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		return fmt.Errorf("failed to create runtime dir: %w", err)
	}

	tempSourcePath := filepath.Join(runtimeDir, "renderer.ts")
	sourceContent := s.adapter.ProdRendererSource()

	if err := os.WriteFile(tempSourcePath, []byte(sourceContent), 0644); err != nil {
		return fmt.Errorf("failed to write temp source: %w", err)
	}

	outfile := filepath.Join(runtimeDir, "bifrost-renderer")
	if os.Getenv("GOOS") == "windows" || (os.Getenv("GOOS") == "" && os.PathSeparator == '\\') {
		outfile += ".exe"
	}

	cmd := exec.Command(
		"bun",
		"build",
		"--compile",
		"--outfile",
		outfile,
		"--no-compile-autoload-dotenv",
		"--no-compile-autoload-bunfig",
		tempSourcePath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		_ = os.Remove(tempSourcePath)
		return fmt.Errorf("bun compile failed: %w", err)
	}

	_ = os.Remove(tempSourcePath)
	return nil
}

func (s *BuildService) parseBuildError(entryName string, err error) BuildError {
	errStr := err.Error()
	lines := strings.Split(errStr, "\n")

	var message string
	var details []string

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if i == 0 {
			message = line
			continue
		}

		details = append(details, line)
	}

	if message == "" && len(details) > 0 {
		message = details[0]
		details = details[1:]
	}

	return BuildError{
		Page:    entryName,
		Message: message,
		Details: details,
	}
}

func (s *BuildService) copyPublicDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if !info.IsDir() {
		return fmt.Errorf("public path is not a directory: %s", src)
	}

	return s.copyDirRecursive(src, dst)
}

// copyDirRecursive uses streaming io.Copy instead of ReadFile/WriteFile to reduce peak memory.
func (s *BuildService) copyDirRecursive(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", src, err)
	}

	if err := os.MkdirAll(dst, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dst, err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := s.copyDirRecursive(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFileStream(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func copyFileStream(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", src, err)
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", dst, err)
	}
	defer func() { _ = dstFile.Close() }()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy file %s: %w", src, err)
	}
	return nil
}
