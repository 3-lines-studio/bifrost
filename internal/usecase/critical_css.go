package usecase

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func (s *BuildService) populateCriticalCSS(ctx context.Context, bifrostDir string, pages []pageMetadata, manifest *core.Manifest) {
	if manifest == nil {
		return
	}
	for i := range pages {
		pm := pages[i]
		entry, ok := manifest.Entries[pm.entryName]
		if !ok || entry.CSS == "" {
			continue
		}

		htmlDoc := s.renderCriticalHTML(ctx, bifrostDir, pm)
		if htmlDoc == "" {
			continue
		}

		cssPath := resolveBuiltAssetPath(bifrostDir, entry.CSS)
		if cssPath == "" {
			continue
		}
		cssBytes, err := os.ReadFile(cssPath)
		if err != nil {
			continue
		}

		entry.CriticalCSS = core.ExtractCriticalCSS(htmlDoc, string(cssBytes), core.DefaultCriticalCSSMaxBytes)
		manifest.Entries[pm.entryName] = entry
	}
}

func (s *BuildService) renderCriticalHTML(ctx context.Context, bifrostDir string, pm pageMetadata) string {
	if s.renderer == nil {
		return ""
	}

	switch pm.config.Mode {
	case core.ModeClientOnly:
		return ""
	case core.ModeStaticPrerender:
		props := map[string]any{}
		if pm.config.StaticDataLoader != nil {
			entries, err := pm.config.StaticDataLoader(ctx)
			if err != nil || len(entries) == 0 {
				return ""
			}
			props = entries[0].Props
		}
		return s.renderCriticalPage(filepath.Join(bifrostDir, "ssr", pm.entryName+"-ssr.js"), props)
	default:
		return s.renderCriticalPage(filepath.Join(bifrostDir, "ssr", pm.entryName+"-ssr.js"), map[string]any{})
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
