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
	if scriptSrc == "" {
		return errors.New("missing script src")
	}

	langAttr := SanitizeHTMLLang(htmlLang)
	classAttr := SanitizeHTMLClass(htmlClass)

	hasCustomTitle := false
	if headHTML != "" {
		hasCustomTitle = containsTitle(headHTML)
	}

	styleTags := RenderStyleTags(criticalCSS, cssHrefs)

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
	if styleTags != "" {
		if _, err := io.WriteString(w, styleTags); err != nil {
			return err
		}
	}

	for _, chunk := range chunks {
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
	if _, err := io.WriteString(w, scriptSrc); err != nil {
		return err
	}
	if _, err := io.WriteString(w, `" />`); err != nil {
		return err
	}

	_, err := io.WriteString(w, "\n  </head>\n  <body>\n    <div id=\"app\">")
	return err
}

// WriteHTMLSuffix writes the closing </div>, props script, deferred scripts, and closing body/html.
func WriteHTMLSuffix(w io.Writer, propsJSON []byte, scriptSrc string, chunks []string) error {
	if scriptSrc == "" {
		return errors.New("missing script src")
	}
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

	for _, chunk := range chunks {
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
	if _, err := io.WriteString(w, scriptSrc); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\" type=\"module\" defer></script>\n  </body>\n</html>\n")
	return err
}

func RenderHTMLShell(bodyHTML string, props map[string]any, scriptSrc string, headHTML string, criticalCSS string, cssHrefs []string, chunks []string, htmlLang string, htmlClass string) (string, error) {
	propsJSON, err := MarshalBifrostPropsJSON(props)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	if err := WriteHTMLPreamble(&sb, headHTML, scriptSrc, criticalCSS, cssHrefs, chunks, htmlLang, htmlClass); err != nil {
		return "", err
	}
	if _, err := sb.WriteString(bodyHTML); err != nil {
		return "", err
	}
	if err := WriteHTMLSuffix(&sb, propsJSON, scriptSrc, chunks); err != nil {
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
	deferNonCritical := strings.TrimSpace(criticalCSS) != ""
	for _, href := range cssHrefs {
		if href == "" {
			continue
		}
		if deferNonCritical {
			sb.WriteString(`<link rel="stylesheet" href="`)
			sb.WriteString(href)
			sb.WriteString(`" media="print" onload="this.media='all'" />`)
			sb.WriteString(`<noscript><link rel="stylesheet" href="`)
			sb.WriteString(href)
			sb.WriteString(`" /></noscript>`)
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
