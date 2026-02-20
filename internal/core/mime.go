package core

import (
	"path/filepath"
	"strings"
)

var contentTypes = map[string]string{
	".css":   "text/css",
	".js":    "application/javascript",
	".json":  "application/json",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".svg":   "image/svg+xml",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".ttf":   "font/ttf",
	".eot":   "application/vnd.ms-fontobject",
	".ico":   "image/x-icon",
}

func GetContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ct, ok := contentTypes[ext]; ok {
		return ct
	}
	return "application/octet-stream"
}
