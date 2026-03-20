package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"html"
	"strings"
)

var emptyPropsJSON = []byte("{}")

func RenderHTMLShell(bodyHTML string, props map[string]any, scriptSrc string, headHTML string, cssHref string, chunks []string, htmlLang string) (string, error) {
	if scriptSrc == "" {
		return "", errors.New("missing script src")
	}

	langAttr := SanitizeHTMLLang(htmlLang)

	hasCustomTitle := false
	if headHTML != "" {
		hasCustomTitle = containsTitle(headHTML)
	}

	var propsJSON []byte
	if len(props) == 0 {
		propsJSON = emptyPropsJSON
	} else {
		var err error
		propsJSON, err = json.Marshal(props)
		if err != nil {
			return "", err
		}
	}

	// Only run escape when the dangerous sequence is present
	if bytes.Contains(propsJSON, []byte("</")) {
		propsJSON = bytes.ReplaceAll(propsJSON, []byte("</"), []byte("<\\/"))
	}

	// Pre-calculate approximate capacity
	const staticLen = 250 // fixed HTML structure overhead
	capacity := staticLen + len(bodyHTML) + len(propsJSON) + len(scriptSrc) + len(headHTML) + len(cssHref)
	if cssHref != "" {
		// non-blocking link + duplicate href inside noscript
		capacity += len(cssHref) + 120
	}
	for _, chunk := range chunks {
		capacity += 55 + len(chunk) // <script src="..." type="module" defer></script>\n
		capacity += 36 + len(chunk) // <link rel="modulepreload" href="..." />
	}
	capacity += 36 + len(scriptSrc) // modulepreload for entry

	var sb strings.Builder
	sb.Grow(capacity)

	sb.WriteString("<!doctype html>\n<html lang=\"")
	sb.WriteString(html.EscapeString(langAttr))
	sb.WriteString("\">\n  <head>\n    ")
	sb.WriteString(`<meta charset="UTF-8" /><meta name="viewport" content="width=device-width, initial-scale=1.0" />`)

	if !hasCustomTitle {
		sb.WriteString("<title>Bifrost</title>")
	}
	if headHTML != "" {
		sb.WriteString(headHTML)
	}
	if cssHref != "" {
		// Not render-blocking for screen; see https://web.dev/articles/defer-non-critical-css
		sb.WriteString(`<link rel="stylesheet" href="`)
		sb.WriteString(cssHref)
		sb.WriteString(`" media="print" onload="this.media='all'" /><noscript><link rel="stylesheet" href="`)
		sb.WriteString(cssHref)
		sb.WriteString(`" /></noscript>`)
	}

	// Discover module graph during head parse instead of after body scan (shorter critical path).
	for _, chunk := range chunks {
		sb.WriteString(`<link rel="modulepreload" href="`)
		sb.WriteString(chunk)
		sb.WriteString(`" />`)
	}
	sb.WriteString(`<link rel="modulepreload" href="`)
	sb.WriteString(scriptSrc)
	sb.WriteString(`" />`)

	sb.WriteString("\n  </head>\n  <body>\n    <div id=\"app\">")
	sb.WriteString(bodyHTML)
	sb.WriteString("</div>\n    <script id=\"__BIFROST_PROPS__\" type=\"application/json\">")
	sb.Write(propsJSON)
	sb.WriteString("</script>\n")

	for _, chunk := range chunks {
		sb.WriteString(`<script src="`)
		sb.WriteString(chunk)
		sb.WriteString("\" type=\"module\" defer></script>\n")
	}

	sb.WriteString("    <script src=\"")
	sb.WriteString(scriptSrc)
	sb.WriteString("\" type=\"module\" defer></script>\n  </body>\n</html>\n")

	return sb.String(), nil
}

// containsTitle does a case-insensitive check for "<title" without allocating a lowercased copy.
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
