package usecase

import (
	"context"

	"github.com/3-lines-studio/bifrost/internal/core"
)

type ExportInput struct {
	Routes []core.PageConfig
}

type ExportOutput struct {
	Pages []StaticPageExport
	Error error
}

type StaticPageExport struct {
	ComponentPath string
	Entries       []StaticPathExport
}

type StaticPathExport struct {
	Path  string
	Props map[string]any
}

type ExportService struct {
	fs FileReader
}

func NewExportService(fs FileReader) *ExportService {
	return &ExportService{
		fs: fs,
	}
}

func (s *ExportService) ExportStatic(ctx context.Context, input ExportInput) ExportOutput {
	var pages []StaticPageExport

	for _, config := range input.Routes {
		if config.Mode != core.ModeStaticPrerender || config.StaticDataLoader == nil {
			continue
		}

		entries, err := config.StaticDataLoader(ctx)
		if err != nil {
			return ExportOutput{
				Error: err,
			}
		}

		var exports []StaticPathExport
		for _, entry := range entries {
			exports = append(exports, StaticPathExport{
				Path:  entry.Path,
				Props: entry.Props,
			})
		}

		pages = append(pages, StaticPageExport{
			ComponentPath: config.ComponentPath,
			Entries:       exports,
		})
	}

	return ExportOutput{
		Pages: pages,
	}
}
