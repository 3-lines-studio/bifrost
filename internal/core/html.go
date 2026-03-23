package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"html"
	"io"
	"strings"
)

var emptyPropsJSON = []byte("{}")

type HTMLDocumentShell struct {
	scriptSrc string
	styleTags string
	chunks    []string
}

func NewHTMLDocumentShell(scriptSrc string, criticalCSS string, cssHrefs []string, chunks []string) (HTMLDocumentShell, error) {
	if scriptSrc == "" {
		return HTMLDocumentShell{}, errors.New("missing script src")
	}
	return HTMLDocumentShell{
		scriptSrc: scriptSrc,
		styleTags: RenderStyleTags(criticalCSS, cssHrefs),
		chunks:    append([]string(nil), chunks...),
	}, nil
}

// MarshalBifrostPropsJSON marshals props for embedding in the __BIFROST_PROPS__ script tag.
func MarshalBifrostPropsJSON(props map[string]any) ([]byte, error) {
	if len(props) == 0 {
		return emptyPropsJSON, nil
	}
	propsJSON, err := json.Marshal(props)
	if err != nil {
		return nil, err
	}
	if bytes.Contains(propsJSON, []byte("</")) {
		propsJSON = bytes.ReplaceAll(propsJSON, []byte("</"), []byte("<\\/"))
	}
	return propsJSON, nil
}

// WriteHTMLPreamble writes from doctype through the opening <div id="app"> (exclusive of body HTML).
func WriteHTMLPreamble(w io.Writer, headHTML string, scriptSrc string, criticalCSS string, cssHrefs []string, chunks []string, htmlLang string, htmlClass string) error {
	shell, err := NewHTMLDocumentShell(scriptSrc, criticalCSS, cssHrefs, chunks)
	if err != nil {
		return err
	}
	return shell.WritePreamble(w, headHTML, htmlLang, htmlClass)
}

// WriteHTMLSuffix writes the closing </div>, props script, deferred scripts, and closing body/html.
func WriteHTMLSuffix(w io.Writer, propsJSON []byte, scriptSrc string, chunks []string) error {
	shell, err := NewHTMLDocumentShell(scriptSrc, "", nil, chunks)
	if err != nil {
		return err
	}
	return shell.WriteSuffix(w, propsJSON)
}

func RenderHTMLShell(bodyHTML string, props map[string]any, scriptSrc string, headHTML string, criticalCSS string, cssHrefs []string, chunks []string, htmlLang string, htmlClass string) (string, error) {
	shell, err := NewHTMLDocumentShell(scriptSrc, criticalCSS, cssHrefs, chunks)
	if err != nil {
		return "", err
	}
	return shell.Render(bodyHTML, props, headHTML, htmlLang, htmlClass)
}

func (s HTMLDocumentShell) WritePreamble(w io.Writer, headHTML string, htmlLang string, htmlClass string) error {
	langAttr := SanitizeHTMLLang(htmlLang)
	classAttr := SanitizeHTMLClass(htmlClass)

	hasCustomTitle := false
	if headHTML != "" {
		hasCustomTitle = containsTitle(headHTML)
	}

	if _, err := io.WriteString(w, "<!doctype html>\n<html lang=\""); err != nil {
		return err
	}
	if _, err := io.WriteString(w, html.EscapeString(langAttr)); err != nil {
		return err
	}
	if _, err := io.WriteString(w, `"`); err != nil {
		return err
	}
	if classAttr != "" {
		if _, err := io.WriteString(w, ` class="`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, html.EscapeString(classAttr)); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `"`); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, ">\n  <head>\n    "); err != nil {
		return err
	}
	if _, err := io.WriteString(w, `<meta charset="UTF-8" /><meta name="viewport" content="width=device-width, initial-scale=1.0" />`); err != nil {
		return err
	}

	if !hasCustomTitle {
		if _, err := io.WriteString(w, "<title>Bifrost</title>"); err != nil {
			return err
		}
	}
	if headHTML != "" {
		if _, err := io.WriteString(w, headHTML); err != nil {
			return err
		}
	}
	if s.styleTags != "" {
		if _, err := io.WriteString(w, s.styleTags); err != nil {
			return err
		}
	}

	for _, chunk := range s.chunks {
		if _, err := io.WriteString(w, `<link rel="modulepreload" href="`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, chunk); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `" />`); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, `<link rel="modulepreload" href="`); err != nil {
		return err
	}
	if _, err := io.WriteString(w, s.scriptSrc); err != nil {
		return err
	}
	if _, err := io.WriteString(w, `" />`); err != nil {
		return err
	}

	_, err := io.WriteString(w, "\n  </head>\n  <body>\n    <div id=\"app\">")
	return err
}

func (s HTMLDocumentShell) WriteSuffix(w io.Writer, propsJSON []byte) error {
	if len(propsJSON) == 0 {
		propsJSON = emptyPropsJSON
	}
	if _, err := io.WriteString(w, "</div>\n    <script id=\"__BIFROST_PROPS__\" type=\"application/json\">"); err != nil {
		return err
	}
	if _, err := w.Write(propsJSON); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "</script>\n"); err != nil {
		return err
	}

	for _, chunk := range s.chunks {
		if _, err := io.WriteString(w, `    <script src="`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, chunk); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "\" type=\"module\" defer></script>\n"); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w, "    <script src=\""); err != nil {
		return err
	}
	if _, err := io.WriteString(w, s.scriptSrc); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\" type=\"module\" defer></script>\n  </body>\n</html>\n")
	return err
}

func (s HTMLDocumentShell) Render(bodyHTML string, props map[string]any, headHTML string, htmlLang string, htmlClass string) (string, error) {
	propsJSON, err := MarshalBifrostPropsJSON(props)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	if err := s.WritePreamble(&sb, headHTML, htmlLang, htmlClass); err != nil {
		return "", err
	}
	if _, err := sb.WriteString(bodyHTML); err != nil {
		return "", err
	}
	if err := s.WriteSuffix(&sb, propsJSON); err != nil {
		return "", err
	}

	return sb.String(), nil
}

func RenderStyleTags(criticalCSS string, cssHrefs []string) string {
	if criticalCSS == "" && len(cssHrefs) == 0 {
		return ""
	}

	var sb strings.Builder
	if criticalCSS != "" {
		sb.WriteString(`<style data-bifrost-critical>`)
		sb.WriteString(sanitizeInlineStyleText(criticalCSS))
		sb.WriteString(`</style>`)
	}
	for _, href := range cssHrefs {
		if href == "" {
			continue
		}
		sb.WriteString(`<link rel="stylesheet" href="`)
		sb.WriteString(href)
		sb.WriteString(`" />`)
	}
	return sb.String()
}

func sanitizeInlineStyleText(css string) string {
	lower := strings.ToLower(css)
	if !strings.Contains(lower, "</style") {
		return css
	}

	var sb strings.Builder
	start := 0
	for {
		idx := strings.Index(lower[start:], "</style")
		if idx == -1 {
			sb.WriteString(css[start:])
			return sb.String()
		}
		idx += start
		sb.WriteString(css[start:idx])
		sb.WriteString(`<\/style`)
		start = idx + len("</style")
	}
}

func containsTitle(s string) bool {
	const needle = "<title"
	nLen := len(needle)
	if len(s) < nLen {
		return false
	}
	for i := 0; i <= len(s)-nLen; i++ {
		if (s[i] == '<') && strings.EqualFold(s[i:i+nLen], needle) {
			return true
		}
	}
	return false
}
