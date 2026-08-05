package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	Path  string `json:"path"`
	Props any    `json:"props"`
}

func WriteStaticBuildExport(w io.Writer, routes []core.Route) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	export := staticBuildExport{
		Version: 1,
		Pages:   make([]staticPageExport, 0),
	}
	pageIndex := make(map[string]int)

	for _, route := range routes {
		config, err := core.PageConfigFromRoute(route)
		if err != nil {
			return err
		}
		if config.Mode != core.ModeStaticPrerender {
			continue
		}

		entries := []core.StaticPathData{{Path: route.Pattern, Props: map[string]any{}}}
		if config.StaticDataLoader != nil {
			var err error
			entries, err = config.StaticDataLoader(ctx)
			if err != nil {
				return fmt.Errorf("failed to load static data for %s: %w", config.ComponentPath, err)
			}
		}

		idx, ok := pageIndex[config.ComponentPath]
		if !ok {
			idx = len(export.Pages)
			pageIndex[config.ComponentPath] = idx
			export.Pages = append(export.Pages, staticPageExport{ComponentPath: config.ComponentPath})
		}
		for _, entry := range entries {
			export.Pages[idx].Entries = append(export.Pages[idx].Entries, staticPathExport{
				Path:  entry.Path,
				Props: entry.Props,
			})
		}
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(export); err != nil {
		return fmt.Errorf("failed to encode export data: %w", err)
	}

	return nil
}

func WriteStaticBuildExportToStdout(routes []core.Route) error {
	return WriteStaticBuildExport(os.Stdout, routes)
}
