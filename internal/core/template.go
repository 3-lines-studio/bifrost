package core

import (
	"path/filepath"
	"strings"
)

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
