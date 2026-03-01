package framework

import (
	_ "embed"
	"github.com/3-lines-studio/bifrost/internal/core"
)

var (
	//go:embed svelte_ssr.txt
	svelteSSRTemplate string

	//go:embed svelte_client_hydration.txt
	svelteClientHydrationTemplate string

	//go:embed svelte_client_only.txt
	svelteClientOnlyTemplate string

	//go:embed assets/svelte_dev.ts
	svelteDevRendererSource string

	//go:embed assets/svelte_prod.ts
	svelteProdRendererSource string
)

type SvelteAdapter struct{}

func NewSvelteAdapter() *SvelteAdapter {
	return &SvelteAdapter{}
}

func (a *SvelteAdapter) Name() string {
	return "svelte"
}

func (a *SvelteAdapter) FileExtension() string {
	return ".svelte"
}

func (a *SvelteAdapter) EntryFileExtension() string {
	return ".ts"
}

func (a *SvelteAdapter) SSREntryTemplate() string {
	return svelteSSRTemplate
}

func (a *SvelteAdapter) ClientEntryTemplate(mode core.PageMode) string {
	switch mode {
	case core.ModeClientOnly:
		return svelteClientOnlyTemplate
	default:
		return svelteClientHydrationTemplate
	}
}

func (a *SvelteAdapter) DevRendererSource() string {
	return svelteDevRendererSource
}

func (a *SvelteAdapter) ProdRendererSource() string {
	return svelteProdRendererSource
}

func (a *SvelteAdapter) BuildPlugins() []string {
	return []string{"bun-plugin-svelte"}
}

func (a *SvelteAdapter) RuntimeImports() []string {
	return []string{
		"svelte",
		"svelte/server",
	}
}
