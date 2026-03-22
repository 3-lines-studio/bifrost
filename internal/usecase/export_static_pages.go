package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/core"
)

// ExportStaticPagesInput carries everything needed to prerender static HTML routes.
type ExportStaticPagesInput struct {
	OutputDir      string
	Routes         []core.Route
	PageConfigs    map[string]*core.PageConfig
	Manifest       *core.Manifest
	AppConfig      *core.Config
	SSBundlePath   func(entryName string) string
	Renderer       Renderer
}

// ExportStaticPages generates static HTML files for StaticPrerender pages and export-manifest.json.
func ExportStaticPages(in ExportStaticPagesInput) error {
	pagesDir := filepath.Join(in.OutputDir, "pages", "routes")
	if err := os.MkdirAll(pagesDir, 0755); err != nil {
		return fmt.Errorf("failed to create pages directory: %w", err)
	}

	exportManifest := &core.Manifest{
		Entries: make(map[string]core.ManifestEntry),
	}

	for _, route := range in.Routes {
		config := core.PageConfigFromRoute(route)
		if config.Mode != core.ModeStaticPrerender {
			continue
		}

		entryName := core.EntryNameForPath(config.ComponentPath)
		ssrBundlePath := in.SSBundlePath(entryName)
		if ssrBundlePath == "" {
			fmt.Printf("Warning: No SSR bundle for %s, skipping\n", route.Pattern)
			continue
		}

		var entries []core.StaticPathData
		if config.StaticDataLoader != nil {
			var err error
			entries, err = config.StaticDataLoader(context.Background())
			if err != nil {
				fmt.Printf("Warning: Failed to load static data for %s: %v, skipping\n", route.Pattern, err)
				continue
			}
		} else {
			entries = []core.StaticPathData{
				{
					Path:  route.Pattern,
					Props: map[string]any{},
				},
			}
		}

		srcEntry := core.ManifestEntry{}
		if in.Manifest != nil {
			srcEntry = in.Manifest.Entries[entryName]
		}

		manifestEntry := core.ManifestEntry{
			Script:       srcEntry.Script,
			CriticalCSS:  srcEntry.CriticalCSS,
			CSS:          srcEntry.CSS,
			CSSFiles:     srcEntry.CSSFiles,
			Chunks:       srcEntry.Chunks,
			Mode:         "static",
			StaticRoutes: make(map[string]string),
		}

		for _, entry := range entries {
			fmt.Printf("Exporting %s...\n", entry.Path)

			appDefault := ""
			if in.AppConfig != nil {
				appDefault = in.AppConfig.DefaultHTMLLang
			}
			lang, htmlClass, propsForReact := core.ResolveHTMLDocumentAttrs(appDefault, config.HTMLLang, config.HTMLClass, entry.Props)

			page, err := in.Renderer.Render(ssrBundlePath, propsForReact)
			if err != nil {
				fmt.Printf("Warning: Failed to render %s: %v, skipping\n", entry.Path, err)
				continue
			}

			criticalCSS := manifestEntry.CriticalCSS
			styleHrefs := core.StylesheetHrefs(manifestEntry.CSS, manifestEntry.CSSFiles)
			if len(styleHrefs) > 0 {
				var fullCSS strings.Builder
				for _, href := range styleHrefs {
					cssPath := filepath.Join(in.OutputDir, filepath.FromSlash(strings.TrimPrefix(href, "/")))
					if cssBytes, err := os.ReadFile(cssPath); err == nil {
						fullCSS.Write(cssBytes)
					}
				}
				if fullCSS.Len() > 0 {
					if extracted := core.ExtractCriticalCSS(page.Head+page.Body, fullCSS.String(), core.DefaultCriticalCSSMaxBytes); extracted != "" {
						criticalCSS = extracted
					}
				}
			}

			html, err := core.RenderHTMLShell(page.Body, propsForReact, manifestEntry.Script, page.Head, criticalCSS, styleHrefs, manifestEntry.Chunks, lang, htmlClass)
			if err != nil {
				fmt.Printf("Warning: Failed to build HTML for %s: %v, skipping\n", entry.Path, err)
				continue
			}

			cleanedRoutePath := path.Clean("/" + entry.Path)
			if strings.Contains(cleanedRoutePath, "..") {
				fmt.Printf("Warning: Unsafe route path %s, skipping\n", entry.Path)
				continue
			}

			htmlPath := filepath.Join(pagesDir, filepath.FromSlash(cleanedRoutePath), "index.html")
			absHTML, err := filepath.Abs(htmlPath)
			if err != nil {
				fmt.Printf("Warning: Failed to resolve path for %s: %v, skipping\n", entry.Path, err)
				continue
			}
			absPages, err := filepath.Abs(pagesDir)
			if err != nil {
				fmt.Printf("Warning: Failed to resolve pages dir: %v, skipping\n", err)
				continue
			}
			if !strings.HasPrefix(absHTML, absPages+string(filepath.Separator)) {
				fmt.Printf("Warning: Route path %s escapes output directory, skipping\n", entry.Path)
				continue
			}

			if err := os.MkdirAll(filepath.Dir(htmlPath), 0755); err != nil {
				fmt.Printf("Warning: Failed to create directory for %s: %v, skipping\n", entry.Path, err)
				continue
			}

			if err := os.WriteFile(htmlPath, []byte(html), 0644); err != nil {
				fmt.Printf("Warning: Failed to write %s: %v, skipping\n", entry.Path, err)
				continue
			}

			normalizedPath := core.NormalizePath(entry.Path)
			manifestEntry.StaticRoutes[normalizedPath] = "/pages/routes" + cleanedRoutePath + "/index.html"
		}

		exportManifest.Entries[entryName] = manifestEntry
	}

	manifestData, err := json.MarshalIndent(exportManifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal export manifest: %w", err)
	}

	manifestPath := filepath.Join(in.OutputDir, "export-manifest.json")
	return os.WriteFile(manifestPath, manifestData, 0644)
}
