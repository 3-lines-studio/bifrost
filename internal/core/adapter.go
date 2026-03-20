package core

type Framework int

const (
	FrameworkReact Framework = iota
)

func (f Framework) String() string {
	switch f {
	case FrameworkReact:
		return "react"
	default:
		return "unknown"
	}
}

func FrameworkFromString(s string) Framework {
	switch s {
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
	SSREntryTemplate(suppressHydrationWarningRoot bool) string
	ClientEntryTemplate(mode PageMode, suppressHydrationWarningRoot bool) string
	DevRendererSource() string
	ProdRendererSource() string
	BuildPlugins() []string
	RuntimeImports() []string
}
