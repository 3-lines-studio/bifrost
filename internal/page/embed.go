package page

import _ "embed"

//go:embed page.html
var pageTemplateSource string

//go:embed error.html
var errorTemplateSource string

func init() {
	SetTemplates(pageTemplateSource, errorTemplateSource)
}
