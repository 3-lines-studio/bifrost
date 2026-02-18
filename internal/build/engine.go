package build

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/3-lines-studio/bifrost/internal/assets"
	"github.com/3-lines-studio/bifrost/internal/cli"
	"github.com/3-lines-studio/bifrost/internal/page"
	bifrost_runtime "github.com/3-lines-studio/bifrost/internal/runtime"
	"github.com/3-lines-studio/bifrost/internal/types"
)

const (
	PublicDir    = "public"
	BifrostDir   = ".bifrost"
	ManifestFile = "manifest.json"
	DistDir      = "dist"
)

type Engine struct {
	client *bifrost_runtime.Client
}

func NewEngine() (*Engine, error) {
	client, err := bifrost_runtime.NewClient()
	if err != nil {
		return nil, err
	}
	return &Engine{client: client}, nil
}

func (e *Engine) Close() error {
	return e.client.Stop()
}

func (e *Engine) BuildProject(mainFile string, originalCwd string) error {
	cli.PrintHeader("Bifrost Build")

	mainDir := filepath.Dir(mainFile)
	if mainDir != "." && mainDir != "" {
		cli.PrintStep(cli.EmojiFolder, "Changing to directory: %s", mainDir)
		if err := os.Chdir(mainDir); err != nil {
			return fmt.Errorf("failed to change to directory %s: %w", mainDir, err)
		}
	}

	cli.PrintStep(cli.EmojiSearch, "Scanning %s for components...", filepath.Base(mainFile))
	pageConfigs, err := scanPages(filepath.Base(mainFile))
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", mainFile, err)
	}

	if len(pageConfigs) == 0 {
		return fmt.Errorf("no NewPage calls found in %s", mainFile)
	}

	cli.PrintSuccess("Found %d component(s)", len(pageConfigs))
	for _, config := range pageConfigs {
		modeStr := "SSR"
		switch config.Mode {
		case types.ModeClientOnly:
			modeStr = "ClientOnly"
		case types.ModeStaticPrerender:
			modeStr = "StaticPrerender"
		}
		cli.PrintFile(config.Path + " [" + modeStr + "]")
	}

	entryDir := filepath.Join(originalCwd, BifrostDir)
	outdir := filepath.Join(entryDir, DistDir)
	ssrDir := filepath.Join(entryDir, "ssr")

	cli.PrintStep(cli.EmojiFolder, "Creating output directories...")
	if err := os.MkdirAll(entryDir, 0755); err != nil {
		return fmt.Errorf("failed to create entry dir: %w", err)
	}
	if err := os.MkdirAll(outdir, 0755); err != nil {
		return fmt.Errorf("failed to create outdir: %w", err)
	}
	if err := os.MkdirAll(ssrDir, 0755); err != nil {
		return fmt.Errorf("failed to create ssr dir: %w", err)
	}
	cli.PrintSuccess("Directories ready")

	cli.PrintStep(cli.EmojiFile, "Generating client entry files...")
	var entryFiles []string
	staticFlags := make(map[string]bool)
	entryToConfig := make(map[string]PageInfo)

	defer func() {
		cli.PrintStep(cli.EmojiGear, "Cleaning up entry files...")
		for _, entryFile := range entryFiles {
			if err := os.Remove(entryFile); err != nil {
				cli.PrintWarning("Failed to remove entry file %s: %v", entryFile, err)
			}
		}
	}()

	for _, config := range pageConfigs {
		entryName := assets.EntryNameForPath(config.Path)
		entryPath := filepath.Join(entryDir, entryName+".tsx")
		staticFlags[entryName] = config.Mode == types.ModeClientOnly || config.Mode == types.ModeStaticPrerender
		entryToConfig[entryPath] = config

		absComponentPath := filepath.Join(originalCwd, config.Path)

		componentImport, err := assets.ComponentImportPath(entryPath, absComponentPath)
		if err != nil {
			return fmt.Errorf("failed to get import path for %s: %w", config.Path, err)
		}

		// ModeClientOnly uses static entry (no hydration)
		// ModeSSR and ModeStaticPrerender use hydration entry
		if config.Mode == types.ModeClientOnly {
			if err := page.WriteStaticClientEntry(entryPath, componentImport); err != nil {
				return fmt.Errorf("failed to write static entry file: %w", err)
			}
		} else {
			if err := page.WriteClientEntry(entryPath, componentImport); err != nil {
				return fmt.Errorf("failed to write entry file: %w", err)
			}
		}
		entryFiles = append(entryFiles, entryPath)
		cli.PrintFile(entryPath)
	}
	cli.PrintSuccess("Generated %d entry file(s)", len(entryFiles))

	cli.PrintStep(cli.EmojiRocket, "Starting Bun renderer...")
	socket := filepath.Join(os.TempDir(), fmt.Sprintf("bifrost-build-%d.sock", os.Getpid()))

	cmd := exec.Command("bun", "run", "--smol", "-")
	cmd.Dir = "."
	cmd.Env = append(os.Environ(), "BIFROST_SOCKET="+socket, "BIFROST_PROD=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = strings.NewReader(bifrost_runtime.BunRendererDevSource)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start bun: %w", err)
	}
	defer cmd.Process.Kill()

	spinner := cli.NewSpinner("Waiting for renderer")
	spinner.Start()
	if err := waitForSocket(socket, 10*time.Second); err != nil {
		spinner.Stop()
		return err
	}
	spinner.Stop()
	cli.PrintSuccess("Renderer ready")

	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socket)
		},
	}
	client := &http.Client{Transport: transport}

	cli.PrintStep(cli.EmojiZap, "Building assets...")

	var successfulEntries []string
	var failedEntries []struct {
		entry string
		info  PageInfo
		err   string
	}

	for _, entryFile := range entryFiles {
		info := entryToConfig[entryFile]
		entryName := assets.EntryNameForPath(info.Path)

		buildSpinner := cli.NewSpinner(fmt.Sprintf("Building %s", entryName))
		buildSpinner.Start()

		reqBody := map[string]interface{}{
			"entrypoints": []string{entryFile},
			"outdir":      outdir,
		}

		jsonBody, err := json.Marshal(reqBody)
		if err != nil {
			buildSpinner.Stop()
			failedEntries = append(failedEntries, struct {
				entry string
				info  PageInfo
				err   string
			}{entryFile, info, fmt.Sprintf("Failed to marshal request: %v", err)})
			continue
		}

		req, err := http.NewRequest("POST", "http://localhost/build", bytes.NewReader(jsonBody))
		if err != nil {
			buildSpinner.Stop()
			failedEntries = append(failedEntries, struct {
				entry string
				info  PageInfo
				err   string
			}{entryFile, info, fmt.Sprintf("Failed to create request: %v", err)})
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			buildSpinner.Stop()
			failedEntries = append(failedEntries, struct {
				entry string
				info  PageInfo
				err   string
			}{entryFile, info, fmt.Sprintf("Build request failed: %v", err)})
			continue
		}

		var result struct {
			OK    bool `json:"ok"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			buildSpinner.Stop()
			failedEntries = append(failedEntries, struct {
				entry string
				info  PageInfo
				err   string
			}{entryFile, info, fmt.Sprintf("Failed to decode response: %v", err)})
			continue
		}
		resp.Body.Close()

		buildSpinner.Stop()

		if result.Error != nil {
			failedEntries = append(failedEntries, struct {
				entry string
				info  PageInfo
				err   string
			}{entryFile, info, result.Error.Message})
			cli.PrintWarning("Failed to build %s: %s", entryName, result.Error.Message)
		} else if !result.OK {
			failedEntries = append(failedEntries, struct {
				entry string
				info  PageInfo
				err   string
			}{entryFile, info, "Build failed"})
			cli.PrintWarning("Failed to build %s", entryName)
		} else {
			successfulEntries = append(successfulEntries, entryFile)
			fmt.Printf("  %s Built %s\n", cli.EmojiCheck, entryName)
		}
	}

	if len(successfulEntries) == 0 {
		return fmt.Errorf("all builds failed")
	}

	if len(failedEntries) > 0 {
		cli.PrintWarning("%d of %d builds failed", len(failedEntries), len(entryFiles))
		for _, failed := range failedEntries {
			cli.PrintFile(fmt.Sprintf("%s: %s", failed.info.Path, failed.err))
		}
	}

	cli.PrintSuccess("Built %d of %d entries", len(successfulEntries), len(entryFiles))

	var ssrEntryFiles []string
	ssrEntryToConfig := make(map[string]PageInfo)

	for _, entryFile := range successfulEntries {
		info := entryToConfig[entryFile]
		// Skip SSR bundles for ClientOnly and StaticPrerender modes
		if info.Mode == types.ModeClientOnly || info.Mode == types.ModeStaticPrerender {
			continue
		}

		entryName := assets.EntryNameForPath(info.Path)
		ssrEntryPath := filepath.Join(entryDir, entryName+"-ssr.tsx")
		ssrEntryToConfig[ssrEntryPath] = info

		absComponentPath := filepath.Join(originalCwd, info.Path)
		componentImport, err := assets.ComponentImportPath(ssrEntryPath, absComponentPath)
		if err != nil {
			cli.PrintWarning("Failed to get import path for SSR %s: %v", info.Path, err)
			continue
		}

		if err := writeServerEntry(ssrEntryPath, componentImport); err != nil {
			cli.PrintWarning("Failed to write SSR entry file for %s: %v", info.Path, err)
			continue
		}

		ssrEntryFiles = append(ssrEntryFiles, ssrEntryPath)
	}

	if len(ssrEntryFiles) > 0 {
		cli.PrintStep(cli.EmojiZap, "Building SSR bundles...")

		for _, ssrEntryFile := range ssrEntryFiles {
			info := ssrEntryToConfig[ssrEntryFile]
			entryName := assets.EntryNameForPath(info.Path)

			buildSpinner := cli.NewSpinner(fmt.Sprintf("Building SSR %s", entryName))
			buildSpinner.Start()

			reqBody := map[string]interface{}{
				"entrypoints": []string{ssrEntryFile},
				"outdir":      ssrDir,
				"target":      "bun",
			}

			jsonBody, err := json.Marshal(reqBody)
			if err != nil {
				buildSpinner.Stop()
				cli.PrintWarning("Failed to marshal SSR request for %s: %v", entryName, err)
				continue
			}

			req, err := http.NewRequest("POST", "http://localhost/build", bytes.NewReader(jsonBody))
			if err != nil {
				buildSpinner.Stop()
				cli.PrintWarning("Failed to create SSR request for %s: %v", entryName, err)
				continue
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				buildSpinner.Stop()
				cli.PrintWarning("SSR build request failed for %s: %v", entryName, err)
				continue
			}

			var result struct {
				OK    bool `json:"ok"`
				Error *struct {
					Message string `json:"message"`
				} `json:"error"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				resp.Body.Close()
				buildSpinner.Stop()
				cli.PrintWarning("Failed to decode SSR response for %s: %v", entryName, err)
				continue
			}
			resp.Body.Close()

			buildSpinner.Stop()

			if result.Error != nil {
				cli.PrintWarning("Failed to build SSR %s: %s", entryName, result.Error.Message)
			} else if !result.OK {
				cli.PrintWarning("Failed to build SSR %s", entryName)
			} else {
				fmt.Printf("  %s Built SSR %s\n", cli.EmojiCheck, entryName)
			}
		}

		cli.PrintStep(cli.EmojiGear, "Cleaning up SSR entry files...")
		for _, ssrEntryFile := range ssrEntryFiles {
			if err := os.Remove(ssrEntryFile); err != nil {
				cli.PrintWarning("Failed to remove SSR entry file %s: %v", ssrEntryFile, err)
			}
		}

		cli.PrintStep(cli.EmojiZap, "Removing unused CSS files from SSR directory...")
		ssrFiles, err := os.ReadDir(ssrDir)
		if err == nil {
			for _, file := range ssrFiles {
				if !file.IsDir() && strings.HasSuffix(file.Name(), ".css") {
					cssPath := filepath.Join(ssrDir, file.Name())
					if err := os.Remove(cssPath); err != nil {
						cli.PrintWarning("Failed to remove CSS file %s: %v", cssPath, err)
					}
					cli.PrintStep("File removed %s", cssPath)
				}
			}
		}
	}

	// Collect static prerender paths that need data loader export
	staticPrerenderWithLoader := []PageInfo{}
	staticPrerenderSimple := []string{}

	for _, config := range pageConfigs {
		if config.Mode == types.ModeStaticPrerender {
			if config.HasStaticDataLoader {
				staticPrerenderWithLoader = append(staticPrerenderWithLoader, config)
			} else {
				staticPrerenderSimple = append(staticPrerenderSimple, config.Path)
			}
		}
	}

	// Generate a temporary manifest to get asset info for HTML generation
	// We'll regenerate it with static routes after all HTML is generated
	componentPaths := make([]string, 0, len(successfulEntries))
	successfulModes := make(map[string]types.PageMode)
	for _, entryFile := range successfulEntries {
		info := entryToConfig[entryFile]
		componentPaths = append(componentPaths, info.Path)
		entryName := assets.EntryNameForPath(info.Path)
		successfulModes[entryName] = info.Mode
	}

	// Create temporary manifest without static routes for HTML generation
	tempStaticRoutes := make(staticRoutesMap)
	man, err := generateManifest(outdir, ssrDir, componentPaths, successfulModes, tempStaticRoutes)
	if err != nil {
		return fmt.Errorf("failed to generate temporary manifest: %w", err)
	}

	// Handle StaticPrerender pages with data loaders - run export and generate multiple pages
	// Build up staticRoutes map for final manifest
	staticRoutes := make(staticRoutesMap)

	if len(staticPrerenderWithLoader) > 0 {
		cli.PrintStep(cli.EmojiFile, "Exporting static data for dynamic paths...")

		exportData, err := runStaticExport(originalCwd)
		if err != nil {
			return fmt.Errorf("failed to export static data: %w", err)
		}

		// Build a map from component path to export entries
		exportMap := make(map[string][]StaticPathExport)
		for _, page := range exportData.Pages {
			exportMap[page.ComponentPath] = page.Entries
		}

		cli.PrintStep(cli.EmojiFile, "Prerendering dynamic static pages...")

		// Validate and generate pages for each component
		allPaths := make(map[string]string) // normalized path -> component

		for _, pageInfo := range staticPrerenderWithLoader {
			entries, ok := exportMap[pageInfo.Path]
			if !ok {
				return fmt.Errorf("no export data found for %s", pageInfo.Path)
			}

			entryName := assets.EntryNameForPath(pageInfo.Path)
			staticRoutes[entryName] = make(map[string]string)

			// Validate paths and check for duplicates
			for _, entry := range entries {
				normalizedPath := normalizeRoutePath(entry.Path)

				if err := validateRoutePath(normalizedPath); err != nil {
					return fmt.Errorf("invalid path for %s: %w", pageInfo.Path, err)
				}

				if existingComponent, exists := allPaths[normalizedPath]; exists {
					return fmt.Errorf("duplicate path %s: already defined by %s", entry.Path, existingComponent)
				}
				allPaths[normalizedPath] = pageInfo.Path
			}

			// Generate HTML for each path and populate staticRoutes
			for _, entry := range entries {
				normalizedPath := normalizeRoutePath(entry.Path)
				routeParts := strings.Split(strings.TrimPrefix(normalizedPath, "/"), "/")

				pageDir := filepath.Join("pages", "routes")
				for _, part := range routeParts {
					if part != "" {
						pageDir = filepath.Join(pageDir, part)
					}
				}
				staticRoutes[entryName][normalizedPath] = "/" + filepath.ToSlash(pageDir) + "/index.html"
			}

			// Generate HTML for each path
			if err := generateDynamicStaticHTMLFiles(entryDir, outdir, pageInfo.Path, entries, man, client, originalCwd); err != nil {
				return fmt.Errorf("failed to generate dynamic static pages for %s: %w", pageInfo.Path, err)
			}

			cli.PrintSuccess("Generated %d pages for %s", len(entries), pageInfo.Path)
		}
	}

	// Handle ClientOnly pages - generate static HTML shells
	clientOnlyPaths := []string{}
	for _, config := range pageConfigs {
		if config.Mode == types.ModeClientOnly {
			clientOnlyPaths = append(clientOnlyPaths, config.Path)
		}
	}

	if len(clientOnlyPaths) > 0 {
		cli.PrintStep(cli.EmojiFile, "Generating static HTML files (ClientOnly)...")

		heads := make(map[string]string)
		for _, componentPath := range clientOnlyPaths {
			// Use absolute path so Bun can resolve from cmd/full working directory
			absComponentPath := filepath.Join(originalCwd, componentPath)
			reqBody := map[string]interface{}{
				"path":  absComponentPath,
				"props": map[string]interface{}{},
			}

			jsonBody, err := json.Marshal(reqBody)
			if err != nil {
				cli.PrintWarning("Failed to marshal render request for %s: %v", componentPath, err)
				continue
			}

			req, err := http.NewRequest("POST", "http://localhost/render", bytes.NewReader(jsonBody))
			if err != nil {
				cli.PrintWarning("Failed to create render request for %s: %v", componentPath, err)
				continue
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				cli.PrintWarning("Failed to render head for %s: %v", componentPath, err)
				continue
			}

			var result struct {
				HTML  string `json:"html"`
				Head  string `json:"head"`
				Error *struct {
					Message string `json:"message"`
				} `json:"error"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				resp.Body.Close()
				cli.PrintWarning("Failed to decode render response for %s: %v", componentPath, err)
				continue
			}
			resp.Body.Close()

			if result.Error != nil {
				cli.PrintWarning("Failed to render head for %s: %s", componentPath, result.Error.Message)
			} else {
				heads[componentPath] = result.Head
			}
		}

		if err := generateClientOnlyHTMLFiles(entryDir, outdir, clientOnlyPaths, heads, man); err != nil {
			return fmt.Errorf("failed to generate static HTML files: %w", err)
		}
		cli.PrintSuccess("Generated %d static HTML file(s)", len(clientOnlyPaths))
	}

	// Handle simple StaticPrerender pages (no data loader)
	if len(staticPrerenderSimple) > 0 {
		cli.PrintStep(cli.EmojiFile, "Prerendering static pages...")

		if err := generatePrerenderedHTMLFiles(entryDir, outdir, staticPrerenderSimple, man, client, originalCwd); err != nil {
			return fmt.Errorf("failed to generate prerendered HTML files: %w", err)
		}
		cli.PrintSuccess("Generated %d prerendered HTML file(s)", len(staticPrerenderSimple))
	}

	// Generate final manifest with static routes
	cli.PrintStep(cli.EmojiPackage, "Generating manifest...")
	man, err = generateManifest(outdir, ssrDir, componentPaths, successfulModes, staticRoutes)
	if err != nil {
		return fmt.Errorf("failed to generate manifest: %w", err)
	}

	manifestPath := filepath.Join(entryDir, ManifestFile)
	manifestData, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}
	cli.PrintFile(manifestPath)
	cli.PrintSuccess("Manifest created")

	cli.PrintStep(cli.EmojiCopy, "Copying public assets...")
	publicSrc := filepath.Join(originalCwd, PublicDir)
	publicDst := filepath.Join(entryDir, PublicDir)
	if err := copyPublicDir(publicSrc, publicDst); err != nil {
		return fmt.Errorf("failed to copy public dir: %w", err)
	}
	cli.PrintSuccess("Assets copied")

	// Check if any pages need SSR (require embedded runtime)
	hasSSR := false
	for _, config := range pageConfigs {
		if config.Mode == types.ModeSSR {
			hasSSR = true
			break
		}
	}

	if hasSSR {
		cli.PrintStep(cli.EmojiZap, "Compiling embedded Bun runtime...")
		if err := compileEmbeddedRuntime(entryDir); err != nil {
			cli.PrintWarning("Failed to compile embedded runtime: %v", err)
			cli.PrintInfo("Applications will need Bun installed at runtime")
		} else {
			cli.PrintSuccess("Embedded runtime ready")
		}
	} else {
		// No SSR pages - remove runtime dir if it exists to prevent stale runtime
		runtimeDir := filepath.Join(entryDir, "runtime")
		if _, err := os.Stat(runtimeDir); err == nil {
			cli.PrintStep(cli.EmojiGear, "Cleaning up runtime directory (no SSR pages)...")
			if err := os.RemoveAll(runtimeDir); err != nil {
				cli.PrintWarning("Failed to remove runtime directory: %v", err)
			}
		}
		cli.PrintInfo("No SSR pages - embedded runtime not needed")
	}

	cli.PrintDone("Build complete! You can now compile your Go binary with embedded assets.")
	return nil
}

func waitForSocket(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for bun socket at %s", path)
}

const serverEntryTemplate = `import * as React from "react";
import { renderToString } from "react-dom/server";
import * as Mod from "{{.ComponentImport}}";

const Component =
  Mod.default ||
  Mod.Page ||
  Object.values(Mod).find((x: any) => typeof x === "function");

const Head = Mod.Head;

export function render(props: Record<string, unknown>): { html: string; head: string } {
  if (!Component) {
    throw new Error("No component export found in {{.ComponentImport}}");
  }

  const el = React.createElement(Component, props);
  const html = renderToString(el);

  let head = "";
  if (Head) {
    try {
      const headEl = React.createElement(Head, props);
      head = renderToString(headEl);
    } catch (headErr) {
      console.error("Error rendering head:", headErr);
    }
  }

  return { html, head };
}
`

func writeServerEntry(path string, componentImport string) error {
	if path == "" {
		return fmt.Errorf("missing entry path")
	}

	if componentImport == "" {
		return fmt.Errorf("missing component import")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tmpl := template.Must(template.New("server-entry").Parse(serverEntryTemplate))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]string{
		"ComponentImport": componentImport,
	}); err != nil {
		return err
	}

	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// staticRoutesMap maps entryName -> map[route]htmlPath for dynamic static pages
type staticRoutesMap map[string]map[string]string

func generateManifest(outdir string, ssrDir string, componentPaths []string, modes map[string]types.PageMode, staticRoutes staticRoutesMap) (*assets.Manifest, error) {
	entries := make(map[string]assets.ManifestEntry)
	chunks := make(map[string]string)

	files, err := os.ReadDir(outdir)
	if err != nil {
		return nil, fmt.Errorf("failed to read outdir: %w", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()

		if strings.HasPrefix(name, "chunk-") && strings.HasSuffix(name, ".js") {
			chunks[name] = "/dist/" + name
		}
	}

	cssFiles := make(map[string]string)
	cssHashToFile := make(map[string]string)

	for _, componentPath := range componentPaths {
		entryName := assets.EntryNameForPath(componentPath)
		var script, css string
		var entryChunks []string

		for _, file := range files {
			if file.IsDir() {
				continue
			}

			name := file.Name()

			if strings.HasPrefix(name, entryName+"-") || strings.HasPrefix(name, entryName+".") {
				if strings.HasSuffix(name, ".js") {
					script = "/dist/" + name
				} else if strings.HasSuffix(name, ".css") {
					css = "/dist/" + name
				}
			}
		}

		if script != "" {
			entryChunks = findEntryChunks(outdir, script, chunks)

			if css != "" {
				css = dedupeCSSFile(outdir, css, cssFiles, cssHashToFile)
			}

			if css == "" && len(cssHashToFile) > 0 {
				for _, sharedCSS := range cssHashToFile {
					css = sharedCSS
					break
				}
			}

			mode := modes[entryName]
			ssrPath := findSSRPath(ssrDir, entryName)

			// Determine mode string and static flag
			modeStr := "ssr"
			isStatic := false
			htmlPath := ""
			routes := staticRoutes[entryName]

			switch mode {
			case types.ModeClientOnly:
				modeStr = "client-only"
				isStatic = true
				htmlPath = "/pages/" + entryName + "/index.html"
			case types.ModeStaticPrerender:
				modeStr = "static-prerender"
				isStatic = true
				// For dynamic static pages, use the routes map
				if routes != nil {
					htmlPath = "" // Route-specific files
				} else {
					htmlPath = "/pages/" + entryName + "/index.html"
				}
			}

			entries[entryName] = assets.ManifestEntry{
				Script:       script,
				CSS:          css,
				Chunks:       entryChunks,
				Static:       isStatic,
				SSR:          ssrPath,
				Mode:         modeStr,
				HTML:         htmlPath,
				StaticRoutes: routes,
			}
		}
	}

	return &assets.Manifest{Entries: entries, Chunks: chunks}, nil
}

func findSSRPath(ssrDir string, entryName string) string {
	if ssrDir == "" {
		return ""
	}
	files, err := os.ReadDir(ssrDir)
	if err != nil {
		return ""
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		name := file.Name()
		if strings.HasPrefix(name, entryName+"-") || strings.HasPrefix(name, entryName+".") {
			if strings.HasSuffix(name, ".js") {
				return "/ssr/" + name
			}
		}
	}
	return ""
}

func dedupeCSSFile(outdir string, cssPath string, cssFiles map[string]string, cssHashToFile map[string]string) string {
	fullPath := filepath.Join(outdir, filepath.Base(cssPath))
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return cssPath
	}

	hash := hashContent(content)

	if existingPath, exists := cssHashToFile[hash]; exists {
		os.Remove(fullPath)
		return existingPath
	}

	cssHashToFile[hash] = cssPath
	return cssPath
}

func hashContent(content []byte) string {
	result := 0
	for _, b := range content {
		result = (result*31 + int(b)) % 1000000007
	}
	return fmt.Sprintf("%d", result)
}

func findEntryChunks(outdir string, entryScript string, allChunks map[string]string) []string {
	var chunks []string

	content, err := os.ReadFile(strings.TrimPrefix(entryScript, "/dist/"))
	if err != nil {
		return chunks
	}

	contentStr := string(content)
	for chunkName := range allChunks {
		if strings.Contains(contentStr, chunkName) {
			chunks = append(chunks, allChunks[chunkName])
		}
	}

	return chunks
}

func generateClientOnlyHTMLFiles(entryDir, outdir string, componentPaths []string, heads map[string]string, man *assets.Manifest) error {
	pagesDir := filepath.Join(entryDir, "pages")
	if err := os.MkdirAll(pagesDir, 0755); err != nil {
		return fmt.Errorf("failed to create pages directory: %w", err)
	}

	for _, componentPath := range componentPaths {
		entryName := assets.EntryNameForPath(componentPath)
		pageDir := filepath.Join(pagesDir, entryName)
		if err := os.MkdirAll(pageDir, 0755); err != nil {
			return fmt.Errorf("failed to create page directory %s: %w", pageDir, err)
		}

		scriptSrc, cssHref, chunks, _, _ := assets.GetAssets(man, entryName)
		htmlPath := filepath.Join(pageDir, "index.html")

		headHTML := heads[componentPath]

		if err := writeStaticHTML(htmlPath, scriptSrc, cssHref, chunks, headHTML, outdir); err != nil {
			return fmt.Errorf("failed to write static HTML for %s: %w", componentPath, err)
		}

		fmt.Printf("  📄 %s\n", htmlPath)
	}

	return nil
}

func writeStaticHTML(htmlPath, scriptSrc, cssHref string, chunks []string, headHTML string, outdir string) error {
	// Use absolute URLs for assets - they are already absolute from manifest (e.g., "/dist/...")
	var cssLink string
	if cssHref != "" {
		cssLink = fmt.Sprintf(`<link rel="stylesheet" href="%s" />`, cssHref)
	}

	head := `<meta charset="UTF-8" /><meta name="viewport" content="width=device-width, initial-scale=1.0" />`
	if headHTML != "" {
		head += headHTML
	}
	head += cssLink

	var chunksHTML strings.Builder
	for _, chunk := range chunks {
		chunksHTML.WriteString(fmt.Sprintf(`<script src="%s" type="module" defer></script>`, chunk))
		chunksHTML.WriteString("\n")
	}

	html := fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    %s
  </head>
  <body>
    <div id="app"></div>
%s    <script src="%s" type="module" defer></script>
  </body>
</html>
`, head, chunksHTML.String(), scriptSrc)

	return os.WriteFile(htmlPath, []byte(html), 0644)
}

func generatePrerenderedHTMLFiles(entryDir, outdir string, componentPaths []string, man *assets.Manifest, client *http.Client, originalCwd string) error {
	pagesDir := filepath.Join(entryDir, "pages")
	if err := os.MkdirAll(pagesDir, 0755); err != nil {
		return fmt.Errorf("failed to create pages directory: %w", err)
	}

	for _, componentPath := range componentPaths {
		entryName := assets.EntryNameForPath(componentPath)
		pageDir := filepath.Join(pagesDir, entryName)
		if err := os.MkdirAll(pageDir, 0755); err != nil {
			return fmt.Errorf("failed to create page directory %s: %w", pageDir, err)
		}

		scriptSrc, cssHref, chunks, _, _ := assets.GetAssets(man, entryName)
		htmlPath := filepath.Join(pageDir, "index.html")

		// Render the component at build time (use absolute path so Bun can resolve from any cwd)
		absComponentPath := filepath.Join(originalCwd, componentPath)
		reqBody := map[string]interface{}{
			"path":  absComponentPath,
			"props": map[string]interface{}{},
		}

		jsonBody, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("failed to marshal render request for %s: %w", componentPath, err)
		}

		req, err := http.NewRequest("POST", "http://localhost/render", bytes.NewReader(jsonBody))
		if err != nil {
			return fmt.Errorf("failed to create render request for %s: %w", componentPath, err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to render %s: %w", componentPath, err)
		}

		var result struct {
			HTML  string `json:"html"`
			Head  string `json:"head"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return fmt.Errorf("failed to decode render response for %s: %w", componentPath, err)
		}
		resp.Body.Close()

		if result.Error != nil {
			return fmt.Errorf("render failed for %s: %s", componentPath, result.Error.Message)
		}

		// Write full HTML with prerendered body (no props for simple static prerender)
		if err := writePrerenderedHTML(htmlPath, scriptSrc, cssHref, chunks, result.Head, result.HTML, nil); err != nil {
			return fmt.Errorf("failed to write prerendered HTML for %s: %w", componentPath, err)
		}

		fmt.Printf("  📄 %s\n", htmlPath)
	}

	return nil
}

func writePrerenderedHTML(htmlPath, scriptSrc, cssHref string, chunks []string, headHTML, bodyHTML string, props map[string]any) error {
	// Use absolute URLs for assets - they are already absolute from manifest (e.g., "/dist/...")
	var cssLink string
	if cssHref != "" {
		cssLink = fmt.Sprintf(`<link rel="stylesheet" href="%s" />`, cssHref)
	}

	head := `<meta charset="UTF-8" /><meta name="viewport" content="width=device-width, initial-scale=1.0" />`
	if headHTML != "" {
		head += headHTML
	}
	head += cssLink

	var chunksHTML strings.Builder
	for _, chunk := range chunks {
		chunksHTML.WriteString(fmt.Sprintf(`<script src="%s" type="module" defer></script>`, chunk))
		chunksHTML.WriteString("\n")
	}

	// Include props for hydration
	var propsScript string
	if props != nil && len(props) > 0 {
		propsJSON, err := json.Marshal(props)
		if err == nil {
			escapedProps := strings.ReplaceAll(string(propsJSON), "</", "<\\/")
			propsScript = fmt.Sprintf(`<script id="__BIFROST_PROPS__" type="application/json">%s</script>`, escapedProps)
		}
	}

	html := fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    %s
  </head>
  <body>
    <div id="app">%s</div>
%s%s    <script src="%s" type="module" defer></script>
  </body>
</html>
`, head, bodyHTML, chunksHTML.String(), propsScript, scriptSrc)

	return os.WriteFile(htmlPath, []byte(html), 0644)
}

func makeRelativeToPages(path string) string {
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	depth := 2
	prefix := strings.Repeat("../", depth)
	return prefix + strings.Join(parts, "/")
}

func copyPublicDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if !info.IsDir() {
		return nil
	}

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return copyFile(path, dstPath)
	})
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, srcInfo.Mode())
}

// StaticPathExport represents a single path entry from export
type StaticPathExport struct {
	Path  string         `json:"path"`
	Props map[string]any `json:"props"`
}

// StaticPageExport represents a single page's static paths from export
type StaticPageExport struct {
	ComponentPath string             `json:"componentPath"`
	Entries       []StaticPathExport `json:"entries"`
}

// StaticBuildExport represents the export format
type StaticBuildExport struct {
	Version int                `json:"version"`
	Pages   []StaticPageExport `json:"pages"`
}

// runStaticExport runs the Go app in export mode and returns the static data
// Note: This function assumes it's called from the main file's directory
// (already chdir'd there by BuildProject)
func runStaticExport(originalCwd string) (*StaticBuildExport, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", ".", "__bifrost_export_static__")

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("export failed: %s", string(exitErr.Stderr))
		}
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("export timed out after 90s; ensure RegisterAssetRoutes is called after all NewPage registrations")
		}
		return nil, fmt.Errorf("failed to run export: %w", err)
	}

	var export StaticBuildExport
	if err := json.Unmarshal(output, &export); err != nil {
		return nil, fmt.Errorf("failed to parse export data: %w", err)
	}

	return &export, nil
}

// normalizeRoutePath normalizes a route path for comparison
func normalizeRoutePath(path string) string {
	// Ensure leading slash
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// Remove trailing slash (except for root)
	if path != "/" && strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}

// validateRoutePath validates that a path is safe and well-formed
func validateRoutePath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must start with /")
	}

	// Check for invalid characters
	if strings.Contains(path, "?") {
		return fmt.Errorf("path cannot contain query string")
	}

	if strings.Contains(path, "#") {
		return fmt.Errorf("path cannot contain fragment")
	}

	if strings.Contains(path, "..") {
		return fmt.Errorf("path cannot contain parent directory references")
	}

	if strings.Contains(path, "*") {
		return fmt.Errorf("path cannot contain wildcards")
	}

	return nil
}

// generateDynamicStaticHTMLFiles generates multiple HTML files for dynamic static paths
func generateDynamicStaticHTMLFiles(entryDir, outdir string, componentPath string, entries []StaticPathExport, man *assets.Manifest, client *http.Client, originalCwd string) error {
	entryName := assets.EntryNameForPath(componentPath)
	scriptSrc, cssHref, chunks, _, _ := assets.GetAssets(man, entryName)

	// Use absolute path for component so Bun can resolve it from any cwd
	absComponentPath := filepath.Join(originalCwd, componentPath)

	pagesDir := filepath.Join(entryDir, "pages")

	for _, entry := range entries {
		// Create directory structure based on path
		normalizedPath := normalizeRoutePath(entry.Path)
		routeParts := strings.Split(strings.TrimPrefix(normalizedPath, "/"), "/")

		pageDir := filepath.Join(pagesDir, "routes")
		for _, part := range routeParts {
			if part != "" {
				pageDir = filepath.Join(pageDir, part)
			}
		}

		if err := os.MkdirAll(pageDir, 0755); err != nil {
			return fmt.Errorf("failed to create page directory %s: %w", pageDir, err)
		}

		htmlPath := filepath.Join(pageDir, "index.html")

		// Render the component with props (use absolute path so Bun can resolve from any cwd)
		reqBody := map[string]interface{}{
			"path":  absComponentPath,
			"props": entry.Props,
		}

		jsonBody, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("failed to marshal render request for %s: %w", entry.Path, err)
		}

		req, err := http.NewRequest("POST", "http://localhost/render", bytes.NewReader(jsonBody))
		if err != nil {
			return fmt.Errorf("failed to create render request for %s: %w", entry.Path, err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to render %s: %w", entry.Path, err)
		}

		var result struct {
			HTML  string `json:"html"`
			Head  string `json:"head"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return fmt.Errorf("failed to decode render response for %s: %w", entry.Path, err)
		}
		resp.Body.Close()

		if result.Error != nil {
			return fmt.Errorf("render failed for %s: %s", entry.Path, result.Error.Message)
		}

		// Write full HTML with prerendered body and props for hydration
		if err := writePrerenderedHTML(htmlPath, scriptSrc, cssHref, chunks, result.Head, result.HTML, entry.Props); err != nil {
			return fmt.Errorf("failed to write prerendered HTML for %s: %w", entry.Path, err)
		}

		fmt.Printf("  📄 %s -> %s\n", entry.Path, htmlPath)
	}

	return nil
}

func compileEmbeddedRuntime(entryDir string) error {
	runtimeDir := filepath.Join(entryDir, "runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		return fmt.Errorf("failed to create runtime dir: %w", err)
	}

	tempSourcePath := filepath.Join(runtimeDir, "renderer.ts")
	sourceContent := bifrost_runtime.BunRendererProdSource
	if err := os.WriteFile(tempSourcePath, []byte(sourceContent), 0644); err != nil {
		return fmt.Errorf("failed to write temp source: %w", err)
	}

	outfile := filepath.Join(runtimeDir, "bifrost-renderer")
	if runtime.GOOS == "windows" {
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
