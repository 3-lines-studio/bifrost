package bifrost

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/3-lines-studio/bifrost/internal/core"
)

type staticBuildExport struct {
	Version int                `json:"version"`
	Pages   []staticPageExport `json:"pages"`
}

type staticPageExport struct {
	ComponentPath string             `json:"componentPath"`
	Entries       []staticPathExport `json:"entries"`
}

type staticPathExport struct {
	Path  string         `json:"path"`
	Props map[string]any `json:"props"`
}

func exportStaticBuildData(app *App) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	export := staticBuildExport{
		Version: 1,
		Pages:   make([]staticPageExport, 0),
	}

	// Build a map from component path to route pattern
	componentToPattern := make(map[string]string)
	for _, route := range app.routes {
		config := buildPageConfig(route)
		if config.Mode == core.ModeStaticPrerender {
			componentToPattern[config.ComponentPath] = route.Pattern
		}
	}

	for componentPath, config := range app.pageConfigs {
		if config.Mode != core.ModeStaticPrerender {
			continue
		}

		var entries []core.StaticPathData
		if config.StaticDataLoader != nil {
			var err error
			entries, err = config.StaticDataLoader(ctx)
			if err != nil {
				return fmt.Errorf("failed to load static data for %s: %w", componentPath, err)
			}
		} else {
			// Static page without data loader - use route pattern with empty props
			pattern := componentToPattern[componentPath]
			if pattern == "" {
				continue // Skip if no route pattern found
			}
			entries = []core.StaticPathData{
				{
					Path:  pattern,
					Props: map[string]any{},
				},
			}
		}

		pageExport := staticPageExport{
			ComponentPath: componentPath,
			Entries:       make([]staticPathExport, len(entries)),
		}

		for i, entry := range entries {
			pageExport.Entries[i] = staticPathExport{
				Path:  entry.Path,
				Props: entry.Props,
			}
		}

		export.Pages = append(export.Pages, pageExport)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(export); err != nil {
		return fmt.Errorf("failed to encode export data: %w", err)
	}

	return nil
}
