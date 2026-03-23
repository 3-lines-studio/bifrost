package framework

import (
	_ "embed"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/adapters/process"
	"github.com/3-lines-studio/bifrost/internal/core"
)

var (
	//go:embed react_ssr.txt
	reactSSRTemplate string

	//go:embed react_client_hydration.txt
	reactClientHydrationTemplate string

	//go:embed react_client_only.txt
	reactClientOnlyTemplate string

)

type ReactAdapter struct{}

func NewReactAdapter() *ReactAdapter {
	return &ReactAdapter{}
}

func DefaultAdapter() core.FrameworkAdapter {
	return NewReactAdapter()
}

func ResolveAdapter(fw core.Framework) core.FrameworkAdapter {
	switch fw {
	case core.FrameworkReact:
		fallthrough
	default:
		return NewReactAdapter()
	}
}

func (a *ReactAdapter) Name() string {
	return "react"
}

func (a *ReactAdapter) FileExtension() string {
	return ".tsx"
}

func (a *ReactAdapter) EntryFileExtension() string {
	return ".tsx"
}

func (a *ReactAdapter) SSREntryTemplate() string {
	return strings.ReplaceAll(reactSSRTemplate, "BIFROST_SSR_PAGE_WRAP", "pageEl")
}

func (a *ReactAdapter) ClientEntryTemplate(mode core.PageMode) string {
	var tmpl string
	switch mode {
	case core.ModeClientOnly:
		tmpl = reactClientOnlyTemplate
	default:
		tmpl = reactClientHydrationTemplate
	}
	var root string
	if mode == core.ModeClientOnly {
		root = `React.createElement(Page, {})`
	} else {
		root = `React.createElement(Page, props)`
	}
	return strings.ReplaceAll(tmpl, "BIFROST_CLIENT_ROOT", root)
}

func (a *ReactAdapter) DevRendererSource() string {
	return process.RuntimeSource(core.ModeDev)
}

func (a *ReactAdapter) ProdRendererSource() string {
	return process.RuntimeSource(core.ModeProd)
}

func (a *ReactAdapter) BuildPlugins() []string {
	return []string{"bun-plugin-tailwind"}
}

func (a *ReactAdapter) RuntimeImports() []string {
	return []string{
		"react",
		"react-dom/server",
		"react-dom/client",
	}
}
