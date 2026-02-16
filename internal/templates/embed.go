package templates

import (
	"embed"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
)

//go:embed all:minimal
var minimalFS embed.FS

//go:embed all:spa
var spaFS embed.FS

//go:embed all:desktop
var desktopFS embed.FS

var validTemplates = []string{"minimal", "spa", "desktop"}

var ErrInvalidTemplate = errors.New("invalid template name")

func GetTemplate(name string) (fs.FS, error) {
	switch name {
	case "minimal":
		return fs.Sub(minimalFS, "minimal")
	case "spa":
		return fs.Sub(spaFS, "spa")
	case "desktop":
		return fs.Sub(desktopFS, "desktop")
	default:
		return nil, ErrInvalidTemplate
	}
}

func GetMinimalTemplate() (fs.FS, error) {
	return GetTemplate("minimal")
}

type TemplateData struct {
	Module string
}

func ProcessFilename(filename string, data TemplateData) (string, bool) {
	if before, ok := strings.CutSuffix(filename, ".tmpl"); ok {
		return before, true
	}
	return filename, false
}

func ProcessContent(content []byte, isTemplate bool, data TemplateData) []byte {
	if !isTemplate {
		return content
	}

	result := string(content)
	result = strings.ReplaceAll(result, "{{.Module}}", data.Module)

	return []byte(result)
}

func DeriveModuleName(projectDir string) string {
	base := filepath.Base(projectDir)
	if base == "." || base == "/" || base == "" {
		return "myapp"
	}
	return base
}
