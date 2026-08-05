package usecase

import (
	"github.com/3-lines-studio/bifrost/internal/adapters/fs"
	"github.com/3-lines-studio/bifrost/internal/core"
	"io"
)

type Renderer interface {
	Render(componentPath string, props any) (core.RenderedPage, error)
	RenderBodyTo(w io.Writer, componentPath string, props any, onHead func(head string) error) error
	Build(entrypoints []string, outdir string, entryNames []string, framework string) (map[string]core.ClientBuildResult, error)
	BuildSSR(entrypoints []string, outdir string, framework string) error
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
