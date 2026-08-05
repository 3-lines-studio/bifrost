package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/core"
)

type ExportStaticPagesInput struct {
	OutputDir    string
	Routes       []core.Route
	Manifest     *core.Manifest
	AppConfig    *core.Config
	SSBundlePath func(entryName string) string
	Renderer     Renderer
}

func ExportStaticPages(in ExportStaticPagesInput) error {
	pagesDir := filepath.Join(in.OutputDir, "pages", "routes")
	if err := os.MkdirAll(pagesDir, 0755); err != nil {
		return fmt.Errorf("failed to create pages directory: %w", err)
	}

	exportManifest := &core.Manifest{
		Entries: make(map[string]core.ManifestEntry),
	}
	cache := stylesheetCache{byKey: make(map[string]string)}
	var exportErrors []error

	for _, route := range in.Routes {
		config, err := core.PageConfigFromRoute(route)
		if err != nil {
			return err
		}
		if config.Mode != core.ModeStaticPrerender {
			continue
		}

		entryName := core.EntryNameForPath(config.ComponentPath)
		if in.SSBundlePath == nil {
			exportErrors = append(exportErrors, fmt.Errorf("static page %s: SSR bundle resolver is not available", route.Pattern))
			continue
		}
		ssrBundlePath := in.SSBundlePath(entryName)
		if ssrBundlePath == "" {
			exportErrors = append(exportErrors, fmt.Errorf("static page %s: no SSR bundle for component %s", route.Pattern, config.ComponentPath))
			continue
		}
		if in.Renderer == nil {
			exportErrors = append(exportErrors, fmt.Errorf("static page %s: renderer is not available", route.Pattern))
			continue
		}

		var entries []core.StaticPathData
		if config.StaticDataLoader != nil {
			var err error
			entries, err = config.StaticDataLoader(context.Background())
			if err != nil {
				exportErrors = append(exportErrors, fmt.Errorf("static page %s: failed to load static data: %w", route.Pattern, err))
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

		manifestEntry, exists := exportManifest.Entries[entryName]
		if !exists {
			srcEntry := core.ManifestEntry{}
			if in.Manifest != nil {
				srcEntry = in.Manifest.Entries[entryName]
			}
			manifestEntry = core.ManifestEntry{
				Script:       srcEntry.Script,
				CriticalCSS:  srcEntry.CriticalCSS,
				CSS:          srcEntry.CSS,
				CSSFiles:     srcEntry.CSSFiles,
				Chunks:       srcEntry.Chunks,
				Mode:         "static",
				StaticRoutes: make(map[string]string),
			}
		}

		for _, entry := range entries {
			normalizedPath, cleanedRoutePath, err := normalizeStaticExportPath(entry.Path)
			if err != nil {
				exportErrors = append(exportErrors, fmt.Errorf("static page %s: %w", route.Pattern, err))
				continue
			}
			if _, exists := manifestEntry.StaticRoutes[normalizedPath]; exists {
				exportErrors = append(exportErrors, fmt.Errorf("static page %s: duplicate output path %q", route.Pattern, normalizedPath))
				continue
			}

			fmt.Printf("Exporting %s...\n", normalizedPath)

			appDefault := ""
			if in.AppConfig != nil {
				appDefault = in.AppConfig.DefaultHTMLLang
			}
			lang, htmlClass, propsForReact := core.ResolveHTMLDocumentAttrs(appDefault, config.HTMLLang, config.HTMLClass, entry.Props)

			page, err := in.Renderer.Render(ssrBundlePath, propsForReact)
			if err != nil {
				exportErrors = append(exportErrors, fmt.Errorf("static page %s: failed to render %s: %w", route.Pattern, normalizedPath, err))
				continue
			}

			criticalCSS := manifestEntry.CriticalCSS
			styleHrefs := core.StylesheetHrefs(manifestEntry.CSS, manifestEntry.CSSFiles)
			if len(styleHrefs) > 0 {
				fullCSS := cache.load(in.OutputDir, styleHrefs)
				if fullCSS != "" {
					if extracted := core.ExtractCriticalCSS(page.Head+page.Body, fullCSS, core.DefaultCriticalCSSMaxBytes); extracted != "" {
						criticalCSS = extracted
					}
				}
			}

			html, err := core.RenderHTMLShell(page.Body, propsForReact, manifestEntry.Script, page.Head, criticalCSS, styleHrefs, manifestEntry.Chunks, lang, htmlClass)
			if err != nil {
				exportErrors = append(exportErrors, fmt.Errorf("static page %s: failed to build HTML for %s: %w", route.Pattern, normalizedPath, err))
				continue
			}

			htmlPath := filepath.Join(pagesDir, filepath.FromSlash(cleanedRoutePath), "index.html")
			absHTML, err := filepath.Abs(htmlPath)
			if err != nil {
				exportErrors = append(exportErrors, fmt.Errorf("static page %s: failed to resolve output path for %s: %w", route.Pattern, normalizedPath, err))
				continue
			}
			absPages, err := filepath.Abs(pagesDir)
			if err != nil {
				exportErrors = append(exportErrors, fmt.Errorf("static page %s: failed to resolve pages directory: %w", route.Pattern, err))
				continue
			}
			if absHTML != absPages && !strings.HasPrefix(absHTML, absPages+string(filepath.Separator)) {
				exportErrors = append(exportErrors, fmt.Errorf("static page %s: output path %q escapes the pages directory", route.Pattern, normalizedPath))
				continue
			}

			if err := os.MkdirAll(filepath.Dir(htmlPath), 0755); err != nil {
				exportErrors = append(exportErrors, fmt.Errorf("static page %s: failed to create output directory for %s: %w", route.Pattern, normalizedPath, err))
				continue
			}

			if err := os.WriteFile(htmlPath, []byte(html), 0644); err != nil {
				exportErrors = append(exportErrors, fmt.Errorf("static page %s: failed to write %s: %w", route.Pattern, normalizedPath, err))
				continue
			}

			manifestPath := "/pages/routes/index.html"
			if cleanedRoutePath != "/" {
				manifestPath = "/pages/routes" + cleanedRoutePath + "/index.html"
			}
			manifestEntry.StaticRoutes[normalizedPath] = manifestPath
		}

		exportManifest.Entries[entryName] = manifestEntry
	}

	manifestData, err := json.MarshalIndent(exportManifest, "", "  ")
	if err != nil {
		exportErrors = append(exportErrors, fmt.Errorf("failed to marshal export manifest: %w", err))
		return errors.Join(exportErrors...)
	}

	manifestPath := filepath.Join(in.OutputDir, "export-manifest.json")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		exportErrors = append(exportErrors, fmt.Errorf("failed to write export manifest: %w", err))
	}
	return errors.Join(exportErrors...)
}

func normalizeStaticExportPath(raw string) (normalized string, cleaned string, err error) {
	if raw == "" {
		return "", "", fmt.Errorf("static output path cannot be empty")
	}
	if strings.Contains(raw, "\\") {
		return "", "", fmt.Errorf("static output path %q must use URL slashes", raw)
	}
	if strings.ContainsAny(raw, "?#") {
		return "", "", fmt.Errorf("static output path %q cannot contain a query or fragment", raw)
	}
	if strings.ContainsAny(raw, "{}") {
		return "", "", fmt.Errorf("static output path %q must be a concrete URL path", raw)
	}

	for segment := range strings.SplitSeq(raw, "/") {
		if segment == "." || segment == ".." {
			return "", "", fmt.Errorf("static output path %q contains an unsafe segment", raw)
		}
	}

	cleaned = path.Clean("/" + strings.TrimLeft(raw, "/"))
	return core.NormalizePath(cleaned), cleaned, nil
}
