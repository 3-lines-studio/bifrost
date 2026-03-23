package usecase

import (
	"io"

	"github.com/3-lines-studio/bifrost/internal/core"
)

// RenderHTMLDocumentFromPage assembles a full HTML document from a rendered React page and resolved artifacts.
func RenderHTMLDocumentFromPage(page core.RenderedPage, props map[string]any, artifacts core.PageArtifacts, htmlLang, htmlClass string) (string, error) {
	return core.RenderHTMLShell(
		page.Body,
		props,
		artifacts.Script,
		page.Head,
		artifacts.CriticalCSS,
		core.StylesheetHrefsFor(artifacts),
		artifacts.Chunks,
		htmlLang,
		htmlClass,
	)
}

// WriteSSRHTMLPreamble writes the HTML preamble using React head output and resolved artifacts.
func WriteSSRHTMLPreamble(w io.Writer, headHTML string, artifacts core.PageArtifacts, htmlLang, htmlClass string) error {
	return core.WriteHTMLPreamble(
		w,
		headHTML,
		artifacts.Script,
		artifacts.CriticalCSS,
		core.StylesheetHrefsFor(artifacts),
		artifacts.Chunks,
		htmlLang,
		htmlClass,
	)
}
