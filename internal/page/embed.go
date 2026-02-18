package page

import (
	_ "embed"
	"html/template"
)

//go:embed page.html
var pageTemplateSource string

//go:embed error.html
var errorTemplateSource string

var (
	PageTemplate  = template.Must(template.New("page").Parse(pageTemplateSource))
	ErrorTemplate = template.Must(template.New("error").Parse(errorTemplateSource))
)
