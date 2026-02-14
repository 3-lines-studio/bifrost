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

// ExportStaticBuildData executes static data loaders and outputs the result as JSON.
// This is called during the build process to collect dynamic static paths.
//
// Returns true if export mode was handled (app should exit), false otherwise.
//
// Usage in main.go:
//
//	r, err := bifrost.New(bifrost.WithAssetsFS(bifrostFS))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer r.Stop()
//
//	// Register your pages...
//	home := r.NewPage("./pages/home.tsx")
//	blog := r.NewPage("./pages/blog.tsx",
//	    bifrost.WithStaticPrerender(),
//	    bifrost.WithStaticDataLoader(func(ctx context.Context) ([]bifrost.StaticPathData, error) {
//	        posts, _ := db.ListPosts()
//	        paths := make([]bifrost.StaticPathData, len(posts))
//	        for i, p := range posts {
//	            paths[i] = bifrost.StaticPathData{
//	                Path: "/blog/" + p.Slug,
//	                Props: map[string]any{"slug": p.Slug, "title": p.Title},
//	            }
//	        }
//	        return paths, nil
//	    }),
//	)
//
//	// Handle export mode (for build process)
//	handled, err := bifrost.ExportStaticBuildData(r)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if handled {
//	    return // Exit after exporting
//	}
//
//	// Normal server startup...
func ExportStaticBuildData(r *Renderer) (bool, error) {
	if os.Getenv("BIFROST_EXPORT_STATIC") != "1" {
		return false, nil
	}

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
			return true, fmt.Errorf("failed to load static data for %s: %w", componentPath, err)
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
		return true, fmt.Errorf("failed to encode export data: %w", err)
	}

	return true, nil
}
