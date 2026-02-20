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

type BuildService struct {
	renderer Renderer
	fs       FileReader
	fw       FileWriter
	cli      CLIOutput
}

func NewBuildService(renderer Renderer, fs FileReader, fw FileWriter, cli CLIOutput) *BuildService {
	return &BuildService{
		renderer: renderer,
		fs:       fs,
		fw:       fw,
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

	s.cli.PrintSuccess("Found %d page(s)", len(pageConfigs))

	for _, config := range pageConfigs {
		modeStr := "SSR"
		switch config.Mode {
		case core.ModeClientOnly:
			modeStr = "ClientOnly"
		case core.ModeStaticPrerender:
			modeStr = "StaticPrerender"
		}
		s.cli.PrintFile(config.ComponentPath + " [" + modeStr + "]")
	}

	// Create output directories
	bifrostDir := filepath.Join(input.OriginalCwd, ".bifrost")
	outdir := filepath.Join(bifrostDir, "dist")
	ssrDir := filepath.Join(bifrostDir, "ssr")
	entriesDir := filepath.Join(bifrostDir, "entries")
	pagesDir := filepath.Join(bifrostDir, "pages")

	s.cli.PrintStep("📁", "Creating output directories...")
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
	s.cli.PrintSuccess("Directories ready")

	// Copy public assets
	publicDir := filepath.Join(input.OriginalCwd, "public")
	publicDestDir := filepath.Join(bifrostDir, "public")
	if err := s.copyPublicDir(publicDir, publicDestDir); err != nil {
		s.cli.PrintWarning("Failed to copy public assets: %v", err)
	}

	// Initialize manifest
	manifest := &core.Manifest{
		Entries: make(map[string]core.ManifestEntry),
	}

	// Build SSR bundles FIRST (for SSR and StaticPrerender pages, skip ClientOnly)
	s.cli.PrintStep("⚡", "Building SSR bundles...")
	var ssrEntryFiles []string

	for _, config := range pageConfigs {
		// Skip ClientOnly pages - they don't need SSR bundles
		if config.Mode == core.ModeClientOnly {
			continue
		}

		entryName := core.EntryNameForPath(config.ComponentPath)
		ssrEntryName := entryName + "-ssr"
		ssrEntryPath := filepath.Join(entriesDir, ssrEntryName+".tsx")

		// Calculate relative import path from entry file to component
		absComponentPath := filepath.Join(input.OriginalCwd, config.ComponentPath)
		importPath, err := s.calculateImportPath(ssrEntryPath, absComponentPath)
		if err != nil {
			s.cli.PrintWarning("Failed to calculate import path for %s: %v", config.ComponentPath, err)
			continue
		}

		// Write SSR entry file
		if err := s.writeSSREntry(ssrEntryPath, importPath); err != nil {
			s.cli.PrintWarning("Failed to write SSR entry for %s: %v", config.ComponentPath, err)
			continue
		}
		ssrEntryFiles = append(ssrEntryFiles, ssrEntryPath)

		s.cli.PrintStep("🔨", "Building SSR %s...", ssrEntryName)
		entrypoints := []string{ssrEntryPath}

		if err := s.renderer.BuildSSR(entrypoints, ssrDir); err != nil {
			s.cli.PrintWarning("Failed to build SSR bundle for %s: %v", entryName, err)
			// Continue with other pages - don't fail the entire build
			continue
		}

		// Add to manifest with SSR field
		manifest.Entries[entryName] = core.ManifestEntry{
			Script: "/dist/" + entryName + ".js",
			CSS:    "/dist/" + entryName + ".css",
			SSR:    "/ssr/" + ssrEntryName + ".js",
			Mode:   "ssr",
		}
	}

	// Generate client entry files
	s.cli.PrintStep("📝", "Generating client entry files...")
	var clientEntryFiles []string
	entryToConfig := make(map[string]core.PageConfig)

	for _, config := range pageConfigs {
		entryName := core.EntryNameForPath(config.ComponentPath)
		entryPath := filepath.Join(entriesDir, entryName+".tsx")
		entryToConfig[entryPath] = config

		// Calculate relative import path from entry file to component
		absComponentPath := filepath.Join(input.OriginalCwd, config.ComponentPath)
		importPath, err := s.calculateImportPath(entryPath, absComponentPath)
		if err != nil {
			s.cli.PrintWarning("Failed to calculate import path for %s: %v", config.ComponentPath, err)
			continue
		}

		// Write entry file based on mode
		if config.Mode == core.ModeClientOnly {
			if err := s.writeClientOnlyEntry(entryPath, importPath); err != nil {
				s.cli.PrintWarning("Failed to write client-only entry: %w", err)
				continue
			}
		} else {
			if err := s.writeHydrationEntry(entryPath, importPath); err != nil {
				s.cli.PrintWarning("Failed to write hydration entry: %w", err)
				continue
			}
		}
		clientEntryFiles = append(clientEntryFiles, entryPath)
		s.cli.PrintFile(entryPath)
	}

	// Build client assets
	s.cli.PrintStep("⚡", "Building client assets...")

	for _, entryFile := range clientEntryFiles {
		config := entryToConfig[entryFile]
		entryName := core.EntryNameForPath(config.ComponentPath)

		s.cli.PrintStep("🔨", "Building %s...", entryName)

		entryNames := []string{entryName}
		entrypoints := []string{entryFile}

		if err := s.renderer.Build(entrypoints, outdir, entryNames); err != nil {
			s.cli.PrintWarning("Failed to build %s: %v", entryName, err)
			continue
		}

		// Update or add to manifest
		modeStr := "ssr"
		if config.Mode == core.ModeClientOnly {
			modeStr = "client"
		} else if config.Mode == core.ModeStaticPrerender {
			modeStr = "static"
		}

		entry := manifest.Entries[entryName]
		entry.Script = "/dist/" + entryName + ".js"
		entry.CSS = "/dist/" + entryName + ".css"
		entry.Mode = modeStr
		// Preserve SSR field if already set
		manifest.Entries[entryName] = entry
	}

	// Generate ClientOnly HTML shells
	s.cli.PrintStep("📄", "Generating ClientOnly HTML shells...")
	for _, config := range pageConfigs {
		if config.Mode != core.ModeClientOnly {
			continue
		}

		entryName := core.EntryNameForPath(config.ComponentPath)
		absComponentPath := filepath.Join(input.OriginalCwd, config.ComponentPath)

		// Try to extract title from component
		title := s.extractTitleFromComponent(absComponentPath)

		// Generate HTML shell
		htmlPath := filepath.Join(pagesDir, entryName+".html")
		if err := s.writeClientOnlyHTML(htmlPath, entryName, title); err != nil {
			s.cli.PrintWarning("Failed to generate HTML shell for %s: %v", entryName, err)
			continue
		}

		// Update manifest with HTML field
		entry := manifest.Entries[entryName]
		entry.HTML = "/pages/" + entryName + ".html"
		manifest.Entries[entryName] = entry

		s.cli.PrintFile(htmlPath)
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
	s.cli.PrintStep("📄", "Building StaticPrerender pages...")
	if err := s.runExportMode(input.OriginalCwd, bifrostDir, manifest, input.MainFile); err != nil {
		s.cli.PrintWarning("Export mode failed: %v", err)
		s.cli.PrintWarning("StaticPrerender pages may not be available")
	}

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
		s.cli.PrintStep("🔧", "Compiling embedded Bun runtime...")
		if err := s.compileEmbeddedRuntime(bifrostDir); err != nil {
			s.cli.PrintWarning("Failed to compile embedded runtime: %v", err)
			s.cli.PrintWarning("Production binary will require Bun to be installed")
		} else {
			s.cli.PrintSuccess("Embedded runtime compiled")
		}
	}

	// Clean up entry files
	s.cli.PrintStep("🧹", "Cleaning up entry files...")
	for _, entryFile := range clientEntryFiles {
		os.Remove(entryFile)
	}
	for _, entryFile := range ssrEntryFiles {
		os.Remove(entryFile)
	}

	s.cli.PrintDone("Build completed successfully")
	return BuildOutput{Success: true}
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

func (s *BuildService) buildStaticPrerenderPages(ctx context.Context, originalCwd string, config core.PageConfig, entryName, pagesDir string, manifest *core.Manifest) error {
	if config.StaticDataLoader == nil {
		return fmt.Errorf("no StaticDataLoader configured")
	}

	// Get all static paths and props
	entries, err := config.StaticDataLoader(ctx)
	if err != nil {
		return fmt.Errorf("failed to load static data: %w", err)
	}

	// Initialize StaticRoutes in manifest
	manifestEntry := manifest.Entries[entryName]
	manifestEntry.StaticRoutes = make(map[string]string)

	// Build SSR bundle for StaticPrerender pages
	entriesDir := filepath.Join(originalCwd, ".bifrost", "entries")
	ssrDir := filepath.Join(originalCwd, ".bifrost", "ssr")
	ssrEntryName := entryName + "-ssr"
	ssrEntryPath := filepath.Join(entriesDir, ssrEntryName+".tsx")

	// Calculate import path
	absComponentPath := filepath.Join(originalCwd, config.ComponentPath)
	importPath, err := s.calculateImportPath(ssrEntryPath, absComponentPath)
	if err != nil {
		return fmt.Errorf("failed to calculate import path: %w", err)
	}

	// Write SSR entry
	if err := s.writeSSREntry(ssrEntryPath, importPath); err != nil {
		return fmt.Errorf("failed to write SSR entry: %w", err)
	}
	defer os.Remove(ssrEntryPath)

	// Build SSR bundle
	s.cli.PrintStep("🔨", "Building SSR bundle for %s...", entryName)
	entrypoints := []string{ssrEntryPath}
	if err := s.renderer.BuildSSR(entrypoints, ssrDir); err != nil {
		return fmt.Errorf("failed to build SSR bundle: %w", err)
	}

	ssrBundlePath := filepath.Join(ssrDir, ssrEntryName+".js")

	// Render each static path
	for _, entry := range entries {
		s.cli.PrintStep("📄", "Rendering %s...", entry.Path)

		// Render the page
		renderedPage, err := s.renderer.Render(ssrBundlePath, entry.Props)
		if err != nil {
			s.cli.PrintWarning("Failed to render %s: %v", entry.Path, err)
			continue
		}

		// Generate full HTML
		html := s.renderFullHTML(renderedPage, entryName, entry.Props)

		// Write to file
		htmlPath := filepath.Join(pagesDir, "routes", entry.Path, "index.html")
		if err := os.MkdirAll(filepath.Dir(htmlPath), 0755); err != nil {
			s.cli.PrintWarning("Failed to create directory for %s: %v", entry.Path, err)
			continue
		}
		if err := os.WriteFile(htmlPath, []byte(html), 0644); err != nil {
			s.cli.PrintWarning("Failed to write HTML for %s: %v", entry.Path, err)
			continue
		}

		// Update manifest
		normalizedPath := core.NormalizePath(entry.Path)
		manifestEntry.StaticRoutes[normalizedPath] = "/pages/routes" + entry.Path + "/index.html"
	}

	manifest.Entries[entryName] = manifestEntry
	return nil
}

func (s *BuildService) runExportMode(originalCwd, bifrostDir string, manifest *core.Manifest, mainFile string) error {
	s.cli.PrintStep("📤", "Running export mode to generate static pages...")

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

	defer os.Remove(binaryPath)

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
	os.Remove(exportManifestPath)

	return nil
}

func (s *BuildService) renderFullHTML(page core.RenderedPage, entryName string, props map[string]any) string {
	// Build props JSON
	propsJSON := "{}"
	if len(props) > 0 {
		data, _ := json.Marshal(props)
		propsJSON = string(data)
	}

	// Generate HTML shell with rendered content
	html := fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />%s
    <link rel="stylesheet" href="/dist/%s.css" />
  </head>
  <body>
    <div id="app">%s</div>
    <script id="__BIFROST_PROPS__" type="application/json">%s</script>
    <script src="/dist/%s.js" type="module" defer></script>
  </body>
</html>`,
		page.Head,
		entryName,
		page.Body,
		propsJSON,
		entryName,
	)

	return html
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
		os.Remove(tempSourcePath)
		return fmt.Errorf("bun compile failed: %w", err)
	}

	os.Remove(tempSourcePath)
	return nil
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
