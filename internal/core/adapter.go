package core

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
	case "react":
		fallthrough
	default:
		return FrameworkReact
	}
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
