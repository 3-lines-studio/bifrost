package usecase

import (
	"github.com/3-lines-studio/bifrost/internal/adapters/fs"
	"github.com/3-lines-studio/bifrost/internal/core"
)

type Renderer interface {
	Render(componentPath string, props map[string]any) (core.RenderedPage, error)
	Build(entrypoints []string, outdir string, entryNames []string) error
	BuildSSR(entrypoints []string, outdir string) error
}

type CLIOutput interface {
	PrintHeader(msg string)
	PrintStep(emoji, msg string, args ...any)
	PrintSuccess(msg string, args ...any)
	PrintWarning(msg string, args ...any)
	PrintError(msg string, args ...any)
	PrintFile(path string)
	PrintDone(msg string)
}

type FileSystem = fs.FileSystem
