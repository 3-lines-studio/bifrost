package usecase

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func (s *BuildService) populateCriticalCSS(ctx context.Context, run *buildRun) {
	if run.manifest == nil {
		return
	}
	for _, page := range run.pages {
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

		var fullCSS strings.Builder
		for _, href := range hrefs {
			cssPath := resolveBuiltAssetPath(run.paths.bifrostDir, href)
			if cssPath == "" {
				continue
			}
			cssBytes, err := os.ReadFile(cssPath)
			if err != nil {
				continue
			}
			fullCSS.Write(cssBytes)
		}
		if fullCSS.Len() == 0 {
			continue
		}

		entry.CriticalCSS = core.ExtractCriticalCSS(htmlDoc, fullCSS.String(), core.DefaultCriticalCSSMaxBytes)
		run.manifest.Entries[page.entryName] = entry
	}
}

func (s *BuildService) renderCriticalHTML(ctx context.Context, run *buildRun, page buildPage) string {
	if s.renderer == nil {
		return ""
	}

	switch page.config.Mode {
	case core.ModeClientOnly:
		return ""
	case core.ModeStaticPrerender:
		props := map[string]any{}
		if page.config.StaticDataLoader != nil {
			entries, err := page.config.StaticDataLoader(ctx)
			if err != nil || len(entries) == 0 {
				return ""
			}
			props = entries[0].Props
		}
		return s.renderCriticalPage(filepath.Join(run.paths.bifrostDir, "ssr", page.entryName+"-ssr.js"), props)
	default:
		return s.renderCriticalPage(filepath.Join(run.paths.bifrostDir, "ssr", page.entryName+"-ssr.js"), map[string]any{})
	}
}

func (s *BuildService) renderCriticalPage(renderPath string, props map[string]any) string {
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
