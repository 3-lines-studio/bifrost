package core

import (
	"path/filepath"
	"strings"
)

func EntryNameForPath(componentPath string) string {
	name := strings.TrimPrefix(componentPath, "./")
	name = strings.TrimPrefix(name, "/")
	name = strings.ReplaceAll(filepath.ToSlash(name), "/", "-")
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if name == "" {
		return "page-entry"
	}
	return name + "-entry"
}
