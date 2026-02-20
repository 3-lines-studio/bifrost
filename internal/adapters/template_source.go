package adapters

import (
	"io/fs"

	"github.com/3-lines-studio/bifrost/internal/templates"
)

type TemplateSource struct{}

func NewTemplateSource() *TemplateSource {
	return &TemplateSource{}
}

func (t *TemplateSource) GetTemplate(name string) (fs.FS, error) {
	return templates.GetTemplate(name)
}
