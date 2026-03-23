package usecase

import (
	_ "embed"
	stdhtml "html"
	"os"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/core"
)

//go:embed clientonly_html_template.txt
var clientOnlyHTMLTemplate string

func (s *BuildService) writeClientOnlyHTML(htmlPath, title, script, criticalCSS string, cssHrefs []string, chunks []string, htmlLang string, htmlClass string) error {
	var chunkLines strings.Builder
	for _, c := range chunks {
		chunkLines.WriteString(`    <script src="`)
		chunkLines.WriteString(c)
		chunkLines.WriteString(`" type="module" defer></script>
`)
	}
	styleTags := core.RenderStyleTags(criticalCSS, cssHrefs)
	cssLink := ""
	if styleTags != "" {
		cssLink = "    " + strings.ReplaceAll(styleTags, "><", ">\n    <") + "\n"
	}
	var modulePreload strings.Builder
	for _, c := range chunks {
		modulePreload.WriteString(`    <link rel="modulepreload" href="`)
		modulePreload.WriteString(c)
		modulePreload.WriteString(`" />
`)
	}
	modulePreload.WriteString(`    <link rel="modulepreload" href="`)
	modulePreload.WriteString(script)
	modulePreload.WriteString(`" />
`)
	classAttr := ""
	if sanitizedClass := core.SanitizeHTMLClass(htmlClass); sanitizedClass != "" {
		classAttr = ` class="` + stdhtml.EscapeString(sanitizedClass) + `"`
	}
	html := clientOnlyHTMLTemplate
	html = strings.ReplaceAll(html, "LANG_PLACEHOLDER", htmlLang)
	html = strings.ReplaceAll(html, "HTML_CLASS_PLACEHOLDER", classAttr)
	html = strings.ReplaceAll(html, "TITLE_PLACEHOLDER", title)
	html = strings.ReplaceAll(html, "CSS_LINK_PLACEHOLDER", cssLink)
	html = strings.ReplaceAll(html, "MODULEPRELOAD_PLACEHOLDER", modulePreload.String())
	html = strings.ReplaceAll(html, "CHUNK_SCRIPTS_PLACEHOLDER", chunkLines.String())
	html = strings.ReplaceAll(html, "SCRIPT_SRC_PLACEHOLDER", script)
	return os.WriteFile(htmlPath, []byte(html), 0644)
}

func (s *BuildService) writeSSREntry(entryPath, importPath string) error {
	return WriteSSREntryFile(s.adapter, entryPath, importPath)
}

func (s *BuildService) writeClientOnlyEntry(entryPath, importPath string) error {
	return WriteClientEntryFile(s.adapter, entryPath, importPath, core.ModeClientOnly)
}

func (s *BuildService) writeHydrationEntry(entryPath, importPath string) error {
	return WriteClientEntryFile(s.adapter, entryPath, importPath, core.ModeSSR)
}
