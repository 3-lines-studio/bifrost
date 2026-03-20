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
	".webp":  "image/webp",
	".svg":   "image/svg+xml",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".ttf":   "font/ttf",
	".eot":   "application/vnd.ms-fontobject",
	".ico":   "image/x-icon",
}

func GetContentType(p string) string {
	ext := filepath.Ext(p)
	if ct, ok := contentTypes[ext]; ok {
		return ct
	}
	// Fallback: lowercase for case-insensitive match
	lower := strings.ToLower(ext)
	if lower != ext {
		if ct, ok := contentTypes[lower]; ok {
			return ct
		}
	}
	return "application/octet-stream"
}
