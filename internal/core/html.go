package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"html"
	"strings"
)

var emptyPropsJSON = []byte("{}")

func RenderHTMLShell(bodyHTML string, props map[string]any, scriptSrc string, headHTML string, criticalCSS string, cssHref string, chunks []string, htmlLang string) (string, error) {
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
	styleTags := RenderStyleTags(criticalCSS, cssHref)
	capacity := staticLen + len(bodyHTML) + len(propsJSON) + len(scriptSrc) + len(headHTML) + len(styleTags)
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
	if styleTags != "" {
		sb.WriteString(styleTags)
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

func RenderStyleTags(criticalCSS string, cssHref string) string {
	if criticalCSS == "" && cssHref == "" {
		return ""
	}

	var sb strings.Builder
	sb.Grow(len(criticalCSS) + len(cssHref) + 120)

	if criticalCSS != "" {
		sb.WriteString(`<style data-bifrost-critical>`)
		sb.WriteString(sanitizeInlineStyleText(criticalCSS))
		sb.WriteString(`</style>`)
	}
	if cssHref != "" {
		sb.WriteString(`<link rel="stylesheet" href="`)
		sb.WriteString(cssHref)
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
	sb.Grow(len(css) + 8)

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
