package usecase

import (
	"context"

	"github.com/3-lines-studio/bifrost/internal/adapters/fs"
	"github.com/3-lines-studio/bifrost/internal/core"
)

type Renderer interface {
	Render(componentPath string, props any) (core.RenderedPage, error)
	Build(entrypoints []string, outdir string, entryNames []string) (map[string]core.ClientBuildResult, error)
	BuildSSR(entrypoints []string, outdir string) error
}

type contextRenderer interface {
	RenderContext(ctx context.Context, componentPath string, props any) (core.RenderedPage, error)
}

func renderWithContext(ctx context.Context, renderer Renderer, componentPath string, props any) (core.RenderedPage, error) {
	if renderer, ok := renderer.(contextRenderer); ok {
		return renderer.RenderContext(ctx, componentPath, props)
	}
	return renderer.Render(componentPath, props)
}

type CLIOutput interface {
	PrintHeader(msg string)
	PrintStep(emoji, msg string, args ...any)
	PrintSuccess(msg string, args ...any)
	PrintWarning(msg string, args ...any)
	PrintError(msg string, args ...any)
	PrintFile(path string)
	PrintDone(msg string)
	Green(text string) string
	Yellow(text string) string
	Red(text string) string
	Gray(text string) string
}

type FileSystem = fs.FileSystem
