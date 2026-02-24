package core

import (
	"encoding/json"
	"fmt"
	"strings"
)

func RenderHTMLShell(bodyHTML string, props map[string]any, scriptSrc string, headHTML string, cssHref string, chunks []string) (string, error) {
	if scriptSrc == "" {
		return "", fmt.Errorf("missing script src")
	}

	title := "Bifrost"
	if headHTML != "" {
		headHTMLLower := strings.ToLower(headHTML)
		if strings.Contains(headHTMLLower, "<title") {
			title = ""
		}
	}

	if props == nil {
		props = map[string]any{}
	}

	propsJSON, err := json.Marshal(props)
	if err != nil {
		return "", err
	}

	escapedProps := strings.ReplaceAll(string(propsJSON), "</", "<\\/")

	cssLink := ""
	if cssHref != "" {
		cssLink = fmt.Sprintf(`<link rel="stylesheet" href="%s" />`, cssHref)
	}

	head := `<meta charset="UTF-8" /><meta name="viewport" content="width=device-width, initial-scale=1.0" />`
	if title != "" {
		head += fmt.Sprintf("<title>%s</title>", title)
	}
	if headHTML != "" {
		head += headHTML
	}
	if cssLink != "" {
		head += cssLink
	}

	var chunksHTML strings.Builder
	for _, chunk := range chunks {
		fmt.Fprintf(&chunksHTML, `<script src="%s" type="module" defer></script>`, chunk)
		chunksHTML.WriteString("\n")
	}

	html := fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    %s
  </head>
  <body>
    <div id="app">%s</div>
    <script id="__BIFROST_PROPS__" type="application/json">%s</script>
%s    <script src="%s" type="module" defer></script>
  </body>
</html>
`, head, bodyHTML, escapedProps, chunksHTML.String(), scriptSrc)

	return html, nil
}
