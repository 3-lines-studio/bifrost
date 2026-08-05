package usecase

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/core"
)

type stylesheetCache struct {
	byKey map[string]string
}

func (s *BuildService) populateCriticalCSS(ctx context.Context, run *buildRun) {
	if run.manifest == nil {
		return
	}
	cache := stylesheetCache{byKey: make(map[string]string)}
	for _, page := range run.pages {
		if page.config.Mode == core.ModeStaticPrerender {
			continue
		}
		entry, ok := run.manifest.Entries[page.entryName]
		if !ok {
			continue
		}
		hrefs := core.StylesheetHrefs(entry.CSS, entry.CSSFiles)
		if len(hrefs) == 0 {
			continue
		}

		htmlDoc := s.renderCriticalHTML(ctx, run, page)
		if htmlDoc == "" {
			continue
		}

		fullCSS := cache.load(run.paths.bifrostDir, hrefs)
		if fullCSS == "" {
			continue
		}

		entry.CriticalCSS = core.ExtractCriticalCSS(htmlDoc, fullCSS, core.DefaultCriticalCSSMaxBytes)
		run.manifest.Entries[page.entryName] = entry
	}
}

func (s *BuildService) renderCriticalHTML(ctx context.Context, run *buildRun, page buildPage) string {
	if s.renderer == nil {
		return ""
	}

	if page.config.Mode == core.ModeClientOnly {
		return ""
	}
	entry, ok := run.manifest.Entries[page.entryName]
	if !ok || entry.SSR == "" {
		return ""
	}
	renderPath := buildSSRRenderPath(run.paths.bifrostDir, entry.SSR)
	if renderPath == "" {
		return ""
	}
	if page.config.Mode == core.ModeStaticPrerender {
		var props any
		if page.config.StaticDataLoader != nil {
			entries, err := page.config.StaticDataLoader(ctx)
			if err != nil || len(entries) == 0 {
				return ""
			}
			props = entries[0].Props
		}
		return s.renderCriticalPage(renderPath, props)
	}
	return s.renderCriticalPage(renderPath, map[string]any{})
}

func buildSSRRenderPath(bifrostDir, manifestPath string) string {
	parts := strings.SplitN(manifestPath, "#", 2)
	rel := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(parts[0], "/")))
	if rel == "." || strings.HasPrefix(rel, "..") {
		return ""
	}
	path := filepath.Join(bifrostDir, rel)
	if len(parts) == 2 && parts[1] != "" {
		path += "#" + parts[1]
	}
	return path
}

func (s *BuildService) renderCriticalPage(renderPath string, props any) string {
	page, err := s.renderer.Render(renderPath, props)
	if err != nil {
		return ""
	}
	return page.Head + page.Body
}

func resolveBuiltAssetPath(bifrostDir string, href string) string {
	if href == "" || !strings.HasPrefix(href, "/") {
		return ""
	}

	rel := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(href, "/")))
	if rel == "." || strings.HasPrefix(rel, "..") {
		return ""
	}
	return filepath.Join(bifrostDir, rel)
}

func (c *stylesheetCache) load(root string, hrefs []string) string {
	if len(hrefs) == 0 {
		return ""
	}
	key := root + "\x00" + strings.Join(hrefs, "\x00")
	if css, ok := c.byKey[key]; ok {
		return css
	}

	var fullCSS strings.Builder
	for _, href := range hrefs {
		cssPath := resolveBuiltAssetPath(root, href)
		if cssPath == "" {
			continue
		}
		cssBytes, err := os.ReadFile(cssPath)
		if err != nil {
			continue
		}
		fullCSS.Write(cssBytes)
	}

	css := fullCSS.String()
	c.byKey[key] = css
	return css
}
