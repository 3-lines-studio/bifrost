package bifrost

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// StaticBuildExport represents the export format for static build data
type StaticBuildExport struct {
	Version int                `json:"version"`
	Pages   []StaticPageExport `json:"pages"`
}

// StaticPageExport represents a single page's static paths
type StaticPageExport struct {
	ComponentPath string             `json:"componentPath"`
	Entries       []StaticPathExport `json:"entries"`
}

// StaticPathExport represents a single path entry
type StaticPathExport struct {
	Path  string         `json:"path"`
	Props map[string]any `json:"props"`
}

// exportStaticBuildData executes static data loaders and outputs the result as JSON.
// This is called during the build process to collect dynamic static paths.
func exportStaticBuildData(r *Renderer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	export := StaticBuildExport{
		Version: 1,
		Pages:   make([]StaticPageExport, 0),
	}

	for componentPath, config := range r.pageConfigs {
		// Only export pages with static data loaders
		if config.StaticDataLoader == nil {
			continue
		}

		entries, err := config.StaticDataLoader(ctx)
		if err != nil {
			return fmt.Errorf("failed to load static data for %s: %w", componentPath, err)
		}

		pageExport := StaticPageExport{
			ComponentPath: componentPath,
			Entries:       make([]StaticPathExport, len(entries)),
		}

		for i, entry := range entries {
			pageExport.Entries[i] = StaticPathExport{
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
