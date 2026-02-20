package usecase

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/adapters/cli"
	"github.com/3-lines-studio/bifrost/internal/adapters/process"
	"github.com/3-lines-studio/bifrost/internal/core"
)

//go:embed ssr_entry_template.txt
var ssrEntryTemplate string

//go:embed clientonly_html_template.txt
var clientOnlyHTMLTemplate string

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
}

func NewBuildService(renderer Renderer, fs FileSystem, cli CLIOutput) *BuildService {
	return &BuildService{
		renderer: renderer,
		fs:       fs,
		cli:      cli,
	}
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

	report := cli.NewBuildReport(s.cli, filepath.Join(input.OriginalCwd, ".bifrost"))
	report.SetPageCount(len(pageConfigs))

	// Create output directories
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

	// Copy public assets
	publicDir := filepath.Join(input.OriginalCwd, "public")
	publicDestDir := filepath.Join(bifrostDir, "public")
	if err := s.copyPublicDir(publicDir, publicDestDir); err != nil {
		report.AddWarning("Public assets", "Failed to copy public assets", []string{err.Error()})
	}

	// Initialize manifest
	manifest := &core.Manifest{
		Entries: make(map[string]core.ManifestEntry),
	}

	// Build SSR bundles FIRST (for SSR and StaticPrerender pages, skip ClientOnly)
	stepSSR := report.StartStep("Building SSR bundles")
	var ssrEntryFiles []string
	ssrErrors := make([]BuildError, 0)

	for _, config := range pageConfigs {
		if config.Mode == core.ModeClientOnly {
			continue
		}

		entryName := core.EntryNameForPath(config.ComponentPath)
		ssrEntryName := entryName + "-ssr"
		ssrEntryPath := filepath.Join(entriesDir, ssrEntryName+".tsx")

		absComponentPath := filepath.Join(input.OriginalCwd, config.ComponentPath)
		importPath, err := s.calculateImportPath(ssrEntryPath, absComponentPath)
		if err != nil {
			ssrErrors = append(ssrErrors, BuildError{
				Page:    config.ComponentPath,
				Message: "Failed to calculate import path",
				Details: []string{err.Error()},
			})
			continue
		}

		if err := s.writeSSREntry(ssrEntryPath, importPath); err != nil {
			ssrErrors = append(ssrErrors, BuildError{
				Page:    config.ComponentPath,
				Message: "Failed to write SSR entry",
				Details: []string{err.Error()},
			})
			continue
		}
		ssrEntryFiles = append(ssrEntryFiles, ssrEntryPath)

		entrypoints := []string{ssrEntryPath}
		if err := s.renderer.BuildSSR(entrypoints, ssrDir); err != nil {
			ssrErrors = append(ssrErrors, s.parseBuildError(entryName, err))
			continue
		}

		manifest.Entries[entryName] = core.ManifestEntry{
			Script: "/dist/" + entryName + ".js",
			CSS:    "/dist/" + entryName + ".css",
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

	// Generate client entry files
	stepClient := report.StartStep("Generating client entry files")
	var clientEntryFiles []string
	entryToConfig := make(map[string]core.PageConfig)
	clientEntryErrors := make([]BuildError, 0)

	for _, config := range pageConfigs {
		entryName := core.EntryNameForPath(config.ComponentPath)
		entryPath := filepath.Join(entriesDir, entryName+".tsx")
		entryToConfig[entryPath] = config

		absComponentPath := filepath.Join(input.OriginalCwd, config.ComponentPath)
		importPath, err := s.calculateImportPath(entryPath, absComponentPath)
		if err != nil {
			clientEntryErrors = append(clientEntryErrors, BuildError{
				Page:    config.ComponentPath,
				Message: "Failed to calculate import path",
				Details: []string{err.Error()},
			})
			continue
		}

		if config.Mode == core.ModeClientOnly {
			if err := s.writeClientOnlyEntry(entryPath, importPath); err != nil {
				clientEntryErrors = append(clientEntryErrors, BuildError{
					Page:    entryName,
					Message: "Failed to write client-only entry",
					Details: []string{err.Error()},
				})
				continue
			}
		} else {
			if err := s.writeHydrationEntry(entryPath, importPath); err != nil {
				clientEntryErrors = append(clientEntryErrors, BuildError{
					Page:    entryName,
					Message: "Failed to write hydration entry",
					Details: []string{err.Error()},
				})
				continue
			}
		}
		clientEntryFiles = append(clientEntryFiles, entryPath)
	}
	report.EndStep(stepClient, len(clientEntryErrors) == 0, "")
	for _, err := range clientEntryErrors {
		report.AddWarning(err.Page, err.Message, err.Details)
	}

	// Build client assets
	stepClientAssets := report.StartStep("Building client assets")
	clientAssetErrors := make([]BuildError, 0)

	for _, entryFile := range clientEntryFiles {
		config := entryToConfig[entryFile]
		entryName := core.EntryNameForPath(config.ComponentPath)

		entryNames := []string{entryName}
		entrypoints := []string{entryFile}

		if err := s.renderer.Build(entrypoints, outdir, entryNames); err != nil {
			clientAssetErrors = append(clientAssetErrors, s.parseBuildError(entryName, err))
			continue
		}

		var modeStr string
		switch config.Mode {
		case core.ModeClientOnly:
			modeStr = "client"
		case core.ModeStaticPrerender:
			modeStr = "static"
		default:
			modeStr = "ssr"
		}

		entry := manifest.Entries[entryName]
		entry.Script = "/dist/" + entryName + ".js"
		entry.CSS = "/dist/" + entryName + ".css"
		entry.Mode = modeStr
		manifest.Entries[entryName] = entry
	}
	report.EndStep(stepClientAssets, len(clientAssetErrors) == 0, "")
	for _, err := range clientAssetErrors {
		report.AddError(err.Page, err.Message, err.Details)
	}

	// Generate ClientOnly HTML shells
	stepHTML := report.StartStep("Generating ClientOnly HTML shells")
	htmlErrors := make([]BuildError, 0)
	for _, config := range pageConfigs {
		if config.Mode != core.ModeClientOnly {
			continue
		}

		entryName := core.EntryNameForPath(config.ComponentPath)
		absComponentPath := filepath.Join(input.OriginalCwd, config.ComponentPath)
		title := s.extractTitleFromComponent(absComponentPath)

		htmlPath := filepath.Join(pagesDir, entryName+".html")
		if err := s.writeClientOnlyHTML(htmlPath, entryName, title); err != nil {
			htmlErrors = append(htmlErrors, BuildError{
				Page:    entryName,
				Message: "Failed to generate HTML shell",
				Details: []string{err.Error()},
			})
			continue
		}

		entry := manifest.Entries[entryName]
		entry.HTML = "/pages/" + entryName + ".html"
		manifest.Entries[entryName] = entry
	}
	report.EndStep(stepHTML, len(htmlErrors) == 0, "")
	for _, err := range htmlErrors {
		report.AddWarning(err.Page, err.Message, err.Details)
	}

	// Write manifest before export mode so export binary can read it
	manifestPath := filepath.Join(bifrostDir, "manifest.json")
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return BuildOutput{Success: false, Error: fmt.Errorf("failed to marshal manifest: %w", err)}
	}
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return BuildOutput{Success: false, Error: fmt.Errorf("failed to write manifest: %w", err)}
	}

	// Build StaticPrerender pages via export mode
	stepExport := report.StartStep("Building StaticPrerender pages")
	if err := s.runExportMode(input.OriginalCwd, bifrostDir, manifest, input.MainFile); err != nil {
		report.AddWarning("StaticPrerender", "Export mode failed", []string{err.Error(), "StaticPrerender pages may not be available"})
	}
	report.EndStep(stepExport, err == nil, "")

	// Re-write manifest after export mode to include static routes
	manifestData, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return BuildOutput{Success: false, Error: fmt.Errorf("failed to marshal manifest after export: %w", err)}
	}
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return BuildOutput{Success: false, Error: fmt.Errorf("failed to write manifest after export: %w", err)}
	}

	// Compile embedded runtime if needed
	needsRuntime := false
	for _, config := range pageConfigs {
		if config.Mode != core.ModeClientOnly {
			needsRuntime = true
			break
		}
	}

	if needsRuntime {
		stepRuntime := report.StartStep("Compiling embedded Bun runtime")
		if err := s.compileEmbeddedRuntime(bifrostDir); err != nil {
			report.AddWarning("Runtime", "Failed to compile embedded runtime", []string{err.Error(), "Production binary will require Bun to be installed"})
			report.EndStep(stepRuntime, false, "")
		} else {
			report.EndStep(stepRuntime, true, "")
		}
	}

	// Clean up entry files
	stepCleanup := report.StartStep("Cleaning up entry files")
	for _, entryFile := range clientEntryFiles {
		_ = os.Remove(entryFile)
	}
	for _, entryFile := range ssrEntryFiles {
		_ = os.Remove(entryFile)
	}
	report.EndStep(stepCleanup, true, "")

	// Render the final report
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
		argIndex := 1 // Component path is the second argument (index 1)

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

		// Extract component path (second argument)
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

		// Detect mode from options
		mode, hasStaticDataLoader := s.detectPageMode(callExpr.Args[argIndex:])

		if !seen[path] {
			seen[path] = true
			configs = append(configs, core.PageConfig{
				ComponentPath:    path,
				Mode:             mode,
				StaticDataLoader: nil, // Will be set at runtime
			})
			_ = hasStaticDataLoader // Mark as used - actual loader bound at runtime
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

	// Ensure path starts with ./ or ../
	if !strings.HasPrefix(relPath, ".") {
		relPath = "./" + relPath
	}

	return relPath, nil
}

func (s *BuildService) extractTitleFromComponent(componentPath string) string {
	// Read component file
	data, err := os.ReadFile(componentPath)
	if err != nil {
		return ""
	}
	content := string(data)

	// Try to find title in Head component
	// Pattern: <title>Content</title>
	titleRegex := regexp.MustCompile(`<title>([^}]+?)</title>`)
	matches := titleRegex.FindStringSubmatch(content)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Try to find title in Head function with template literal
	// Pattern: <title>{`Content`}</title>
	titleTemplateRegex := regexp.MustCompile(`<title>\{` + "`" + `([^}]+?)` + "`" + `\}</title>`)
	matches = titleTemplateRegex.FindStringSubmatch(content)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	return ""
}

func (s *BuildService) writeClientOnlyHTML(htmlPath, entryName, title string) error {
	html := clientOnlyHTMLTemplate
	html = strings.ReplaceAll(html, "TITLE_PLACEHOLDER", title)
	html = strings.ReplaceAll(html, "ENTRY_NAME", entryName)

	return os.WriteFile(htmlPath, []byte(html), 0644)
}

func (s *BuildService) runExportMode(originalCwd, bifrostDir string, manifest *core.Manifest, mainFile string) error {
	// Build the user's binary
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

	// Make binary executable
	if err := os.Chmod(binaryPath, 0755); err != nil {
		return fmt.Errorf("failed to make binary executable: %w", err)
	}

	defer func() { _ = os.Remove(binaryPath) }()

	// Run the binary in export mode
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

	// Merge export manifest into main manifest
	exportManifestPath := filepath.Join(bifrostDir, "export-manifest.json")
	exportData, err := os.ReadFile(exportManifestPath)
	if err != nil {
		return fmt.Errorf("failed to read export manifest: %w", err)
	}

	var exportManifest core.Manifest
	if err := json.Unmarshal(exportData, &exportManifest); err != nil {
		return fmt.Errorf("failed to parse export manifest: %w", err)
	}

	// Merge StaticRoutes into main manifest
	for entryName, entry := range exportManifest.Entries {
		if existing, ok := manifest.Entries[entryName]; ok {
			existing.StaticRoutes = entry.StaticRoutes
			manifest.Entries[entryName] = existing
		} else {
			manifest.Entries[entryName] = entry
		}
	}

	// Clean up export manifest
	_ = os.Remove(exportManifestPath)

	return nil
}

func (s *BuildService) writeSSREntry(entryPath, importPath string) error {
	content := strings.ReplaceAll(ssrEntryTemplate, "COMPONENT_PATH", importPath)
	return os.WriteFile(entryPath, []byte(content), 0644)
}

func (s *BuildService) writeClientOnlyEntry(entryPath, importPath string) error {
	content := fmt.Sprintf(`import React from "react";
import { createRoot } from "react-dom/client";
import { Page } from "%s";

const container = document.getElementById("app");
if (container) {
	const root = createRoot(container);
	root.render(React.createElement(Page, {}));
}
`, importPath)

	return os.WriteFile(entryPath, []byte(content), 0644)
}

func (s *BuildService) writeHydrationEntry(entryPath, importPath string) error {
	content := fmt.Sprintf(`import React from "react";
import { hydrateRoot } from "react-dom/client";
import { Page } from "%s";

function getProps() {
	const script = document.getElementById("__BIFROST_PROPS__");
	if (script) {
		try {
			return JSON.parse(script.textContent || "{}");
		} catch (e) {
			console.error("Failed to parse props:", e);
		}
	}
	return {};
}

const container = document.getElementById("app");
if (container) {
	const props = getProps();
	hydrateRoot(container, React.createElement(Page, props));
}
`, importPath)

	return os.WriteFile(entryPath, []byte(content), 0644)
}

func (s *BuildService) compileEmbeddedRuntime(bifrostDir string) error {
	runtimeDir := filepath.Join(bifrostDir, "runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		return fmt.Errorf("failed to create runtime dir: %w", err)
	}

	tempSourcePath := filepath.Join(runtimeDir, "renderer.ts")
	// Use the production renderer source from the process package
	sourceContent := process.BunRendererProdSource

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
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", srcPath, err)
			}

			if err := os.WriteFile(dstPath, data, 0644); err != nil {
				return fmt.Errorf("failed to write file %s: %w", dstPath, err)
			}
		}
	}

	return nil
}
