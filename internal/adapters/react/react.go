package react

import (
	_ "embed"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/adapters/process"
	"github.com/3-lines-studio/bifrost/internal/core"
)

const EntryFileExtension = ".tsx"

var (
	//go:embed react_ssr.txt
	ssrTemplate string

	//go:embed react_client_hydration.txt
	clientHydrationTemplate string

	//go:embed react_client_only.txt
	clientOnlyTemplate string
)

func RuntimeSource(mode core.Mode) string {
	return process.RuntimeSource(mode)
}

func SSREntryTemplate() string {
	return strings.ReplaceAll(ssrTemplate, "BIFROST_SSR_PAGE_WRAP", "pageEl")
}

func ClientEntryTemplate(mode core.PageMode) string {
	if mode == core.ModeClientOnly {
		return strings.ReplaceAll(clientOnlyTemplate, "BIFROST_CLIENT_ROOT", `React.createElement(Page, {})`)
	}
	return strings.ReplaceAll(clientHydrationTemplate, "BIFROST_CLIENT_ROOT", `React.createElement(Page, props)`)
}
