package usecase

import (
	"io/fs"

	"github.com/3-lines-studio/bifrost/internal/core"
)

type FileReader interface {
	ReadFile(path string) ([]byte, error)
	ReadDir(path string) ([]fs.DirEntry, error)
	FileExists(path string) bool
}

type FileWriter interface {
	WriteFile(path string, data []byte, perm fs.FileMode) error
	MkdirAll(path string, perm fs.FileMode) error
	Remove(path string) error
}

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

type TemplateSource interface {
	GetTemplate(name string) (fs.FS, error)
}
