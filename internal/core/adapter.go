package core

import (
	"path/filepath"
	"strings"
)

type Framework int

const (
	FrameworkReact Framework = iota
)

func (f Framework) String() string {
	return "react"
}

func FrameworkFromString(s string) Framework {
	return FrameworkReact
}

func FrameworkFromExtension(ext string) Framework {
	return FrameworkReact
}

func FrameworkFromComponentPath(path string) Framework {
	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}
	return FrameworkFromExtension(filepath.Ext(path))
}

type FrameworkAdapter interface {
	Name() string
	FileExtension() string
	EntryFileExtension() string
	SSREntryTemplate() string
	ClientEntryTemplate(mode PageMode) string
	DevRendererSource() string
	ProdRendererSource() string
	BuildPlugins() []string
	RuntimeImports() []string
}
