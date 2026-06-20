package framework

import (
	_ "embed"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/adapters/process"
	"github.com/3-lines-studio/bifrost/internal/core"
)

var (
	//go:embed svelte_ssr.txt
	svelteSSRTemplate string

	//go:embed svelte_client_hydration.txt
	svelteClientHydrationTemplate string

	//go:embed svelte_client_only.txt
	svelteClientOnlyTemplate string
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
	var tmpl string
	switch mode {
	case core.ModeClientOnly:
		tmpl = svelteClientOnlyTemplate
	default:
		tmpl = svelteClientHydrationTemplate
	}
	root := "Page"
	return strings.ReplaceAll(tmpl, "BIFROST_CLIENT_ROOT", root)
}

func (a *SvelteAdapter) DevRendererSource() string {
	return process.RuntimeSource(core.ModeDev)
}

func (a *SvelteAdapter) ProdRendererSource() string {
	return process.RuntimeSource(core.ModeProd)
}

func (a *SvelteAdapter) BuildPlugins() []string {
	return []string{"bun-plugin-tailwind"}
}

func (a *SvelteAdapter) RuntimeImports() []string {
	return []string{
		"svelte/compiler",
		"svelte/server",
		"svelte",
	}
}
