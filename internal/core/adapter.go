package core

import (
	"path/filepath"
	"strings"
)

type Framework int

const (
	FrameworkReact Framework = iota
	FrameworkSvelte
)

func (f Framework) String() string {
	switch f {
	case FrameworkReact:
		return "react"
	case FrameworkSvelte:
		return "svelte"
	default:
		return "unknown"
	}
}

func FrameworkFromString(s string) Framework {
	switch s {
	case "svelte":
		return FrameworkSvelte
	default:
		return FrameworkReact
	}
}

func FrameworkFromExtension(ext string) Framework {
	switch ext {
	case ".svelte":
		return FrameworkSvelte
	default:
		return FrameworkReact
	}
}

func FrameworkFromComponentPath(path string) Framework {
	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}
	if strings.HasSuffix(path, ".svelte.ts") || strings.HasSuffix(path, ".svelte.js") || strings.HasSuffix(path, ".svelte") {
		return FrameworkSvelte
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
