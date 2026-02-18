package bifrost

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
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

	for componentPath, config := range app.pageConfigs {
		if config.StaticDataLoader == nil {
			continue
		}

		entries, err := config.StaticDataLoader(ctx)
		if err != nil {
			return fmt.Errorf("failed to load static data for %s: %w", componentPath, err)
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
