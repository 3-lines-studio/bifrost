package framework

import (
	_ "embed"
	"github.com/3-lines-studio/bifrost/internal/core"
)

var (
	//go:embed react_ssr.txt
	reactSSRTemplate string

	//go:embed react_client_hydration.txt
	reactClientHydrationTemplate string

	//go:embed react_client_only.txt
	reactClientOnlyTemplate string

	//go:embed assets/react_dev.ts
	reactDevRendererSource string

	//go:embed assets/react_prod.ts
	reactProdRendererSource string
)

type ReactAdapter struct{}

func NewReactAdapter() *ReactAdapter {
	return &ReactAdapter{}
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
	return reactSSRTemplate
}

func (a *ReactAdapter) ClientEntryTemplate(mode core.PageMode) string {
	switch mode {
	case core.ModeClientOnly:
		return reactClientOnlyTemplate
	default:
		return reactClientHydrationTemplate
	}
}

func (a *ReactAdapter) DevRendererSource() string {
	return reactDevRendererSource
}

func (a *ReactAdapter) ProdRendererSource() string {
	return reactProdRendererSource
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
