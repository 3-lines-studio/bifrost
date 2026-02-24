package core

import (
	"fmt"
	"path/filepath"
	"strings"
)

func NormalizePath(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if path != "/" && strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}

func ValidateRoutePath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must start with /")
	}

	if strings.Contains(path, "?") {
		return fmt.Errorf("path cannot contain query string")
	}

	if strings.Contains(path, "#") {
		return fmt.Errorf("path cannot contain fragment")
	}

	if strings.Contains(path, "..") {
		return fmt.Errorf("path cannot contain parent directory references")
	}

	if strings.Contains(path, "*") {
		return fmt.Errorf("path cannot contain wildcards")
	}

	return nil
}

type EntryPaths struct {
	EntryDir  string
	Outdir    string
	EntryName string
	EntryPath string
}

func CalculateEntryPaths(componentPath string) EntryPaths {
	entryDir := ".bifrost"
	outdir := filepath.Join(entryDir, "dist")
	entryName := EntryNameForPath(componentPath)
	entryPath := filepath.Join(entryDir, entryName+".tsx")

	return EntryPaths{
		EntryDir:  entryDir,
		Outdir:    outdir,
		EntryName: entryName,
		EntryPath: entryPath,
	}
}

func ComponentImportPath(entryPath string, componentPath string) (string, error) {
	from := filepath.Dir(entryPath)
	rel, err := filepath.Rel(from, componentPath)
	if err != nil {
		return "", err
	}

	rel = strings.TrimSuffix(rel, filepath.Ext(rel))
	rel = filepath.ToSlash(rel)
	if strings.HasPrefix(rel, ".") {
		return rel, nil
	}

	return "./" + rel, nil
}
