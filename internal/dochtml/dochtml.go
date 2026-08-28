package dochtml

import (
	"bytes"
	"encoding/json"
	"errors"
	"html"
	"io"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

const (
	// AssetPrefix is the HTTP prefix under which built assets are served.
	AssetPrefix = "/_bifrost/"
	// DevPrefix is the HTTP prefix under which the Vite development server is
	// proxied during development. It is Vite's base path, so proxied request
	// paths pass through unchanged.
	DevPrefix = "/_bifrost/dev/"
	// defaultDocumentOpen is the open tag for the default English document.
	defaultDocumentOpen = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">`
)

// Shell is an immutable HTML document shell for one client asset set. The
// streaming runtime uses WritePreamble and WriteSuffix around body frames;
// the build command uses Render to assemble a complete static document.
type Shell struct {
	afterHead string
	suffix    string
}

// NewShell builds the shell for the given client assets.
func NewShell(assets protocol.AssetSet) (Shell, error) {
	if assets.Entry.Path == "" {
		return Shell{}, errors.New("bifrost: missing client entry")
	}
	var after strings.Builder
	for _, style := range assets.Styles {
		after.WriteString(`<link rel="stylesheet" href="`)
		after.WriteString(html.EscapeString(FrontendAssetURL(style.Path)))
		after.WriteString(`">`)
	}
	for _, imported := range assets.Imports {
		after.WriteString(`<link rel="modulepreload" fetchpriority="low" href="`)
		after.WriteString(html.EscapeString(FrontendAssetURL(imported.Path)))
		after.WriteString(`">`)
	}
	entryURL := FrontendAssetURL(assets.Entry.Path)
	if strings.HasPrefix(entryURL, DevPrefix) {
		after.WriteString(`<script type="module">import RefreshRuntime from "` + html.EscapeString(DevPrefix+"@react-refresh") + `";RefreshRuntime.injectIntoGlobalHook(window);window.$RefreshReg$=()=>{};window.$RefreshSig$=()=>type=>type;window.__vite_plugin_react_preamble_installed__=true;</script>`)
		after.WriteString(`<script type="module" src="` + html.EscapeString(DevPrefix+"@vite/client") + `"></script>`)
	}
	after.WriteString(`<link rel="modulepreload" fetchpriority="low" href="`)
	after.WriteString(html.EscapeString(entryURL))
	after.WriteString(`"></head><body><div id="app">`)

	var suffix strings.Builder
	suffix.WriteString(`</div><script id="__BIFROST_PROPS__" type="application/json">`)
	suffix.WriteString("<!--BIFROST:PROPS-->")
	suffix.WriteString(`</script><script type="module" src="`)
	suffix.WriteString(html.EscapeString(entryURL))
	suffix.WriteString(`"></script></body></html>`)

	return Shell{afterHead: after.String(), suffix: suffix.String()}, nil
}

// FrontendAssetURL converts a build artifact path into a browser URL. Root-
// relative URLs pass through for the proxied Vite development server, and
// absolute URLs pass through for external development servers.
func FrontendAssetURL(value string) string {
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	return AssetPrefix + value
}

// Open writes the opening tags for a document with the given attributes.
func Open(document protocol.DocumentAttributes) string {
	var result strings.Builder
	result.WriteString(`<!doctype html><html lang="`)
	result.WriteString(html.EscapeString(document.Lang))
	result.WriteByte('"')
	if document.Class != "" {
		result.WriteString(` class="`)
		result.WriteString(html.EscapeString(document.Class))
		result.WriteByte('"')
	}
	if document.Dir != "" {
		result.WriteString(` dir="`)
		result.WriteString(html.EscapeString(document.Dir))
		result.WriteByte('"')
	}
	result.WriteString(`><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">`)
	return result.String()
}

// WritePreamble writes the document opening, dynamic head, and the fixed
// head/body transition.
func (s Shell) WritePreamble(w io.Writer, head []byte, document protocol.DocumentAttributes) error {
	opening := defaultDocumentOpen
	if document != (protocol.DocumentAttributes{Lang: "en"}) {
		opening = Open(document)
	}
	if _, err := io.WriteString(w, opening); err != nil {
		return err
	}
	if len(head) > 0 {
		if _, err := w.Write(head); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, s.afterHead)
	return err
}

// WriteSuffix writes the closing body and hydration scripts.
func (s Shell) WriteSuffix(w io.Writer, props json.RawMessage) error {
	before, after, ok := strings.Cut(s.suffix, "<!--BIFROST:PROPS-->")
	if !ok {
		return errors.New("bifrost: invalid document shell")
	}
	if _, err := io.WriteString(w, before); err != nil {
		return err
	}
	if _, err := w.Write(props); err != nil {
		return err
	}
	_, err := io.WriteString(w, after)
	return err
}

// Render assembles a complete document from pre-rendered head and body bytes.
func (s Shell) Render(w io.Writer, head, body []byte, props json.RawMessage, document protocol.DocumentAttributes) error {
	if err := s.WritePreamble(w, head, document); err != nil {
		return err
	}
	if len(body) > 0 {
		if _, err := w.Write(body); err != nil {
			return err
		}
	}
	return s.WriteSuffix(w, props)
}

// ClientDocument assembles a mount-only document with empty props.
func (s Shell) ClientDocument(props json.RawMessage) ([]byte, error) {
	var output bytes.Buffer
	if err := s.Render(&output, nil, nil, props, protocol.DocumentAttributes{Lang: "en"}); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
