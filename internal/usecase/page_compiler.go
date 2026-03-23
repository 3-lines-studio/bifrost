package usecase

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/core"
)

// AbsoluteComponentPath resolves a page component path from the project working directory.
func AbsoluteComponentPath(cwd, componentPath string) string {
	p := strings.TrimSpace(componentPath)
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "./") && !strings.HasPrefix(p, string(filepath.Separator)) && !filepath.IsAbs(p) {
		p = "./" + p
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	if strings.HasPrefix(p, "./") {
		return filepath.Join(cwd, filepath.Clean(p[2:]))
	}
	return filepath.Join(cwd, filepath.Clean(p))
}

// CalculateImportPath returns a relative import path from the entry file to the absolute component path.
func CalculateImportPath(entryPath, absComponentPath string) (string, error) {
	entryDir := filepath.Dir(entryPath)
	relPath, err := filepath.Rel(entryDir, absComponentPath)
	if err != nil {
		return "", err
	}
	if relPath == "." {
		return "./", nil
	}
	if !strings.HasPrefix(relPath, ".") {
		relPath = "./" + relPath
	}
	return relPath, nil
}

// WriteSSREntryFile writes the framework SSR entry template with COMPONENT_PATH replaced.
func WriteSSREntryFile(adapter core.FrameworkAdapter, entryPath, importPath string) error {
	content := strings.ReplaceAll(adapter.SSREntryTemplate(), "COMPONENT_PATH", importPath)
	return os.WriteFile(entryPath, []byte(content), 0o644)
}

// WriteClientEntryFile writes the client/hydration entry for the given page mode.
func WriteClientEntryFile(adapter core.FrameworkAdapter, entryPath, importPath string, mode core.PageMode) error {
	var tmpl string
	if mode == core.ModeClientOnly {
		tmpl = adapter.ClientEntryTemplate(core.ModeClientOnly)
	} else {
		tmpl = adapter.ClientEntryTemplate(core.ModeSSR)
	}
	content := strings.ReplaceAll(tmpl, "COMPONENT_PATH", importPath)
	return os.WriteFile(entryPath, []byte(content), 0o644)
}

// CompileDevPageOnDemand writes client + SSR entry files under .bifrost/entries and runs
// client Build and SSR BuildSSR. Used by the dev server first-request setup path.
func CompileDevPageOnDemand(renderer Renderer, cwd string, entryName string, config core.PageConfig, adapter core.FrameworkAdapter) error {
	if renderer == nil {
		return fmt.Errorf("renderer is nil")
	}
	if adapter == nil {
		return fmt.Errorf("adapter is nil")
	}

	entryDir := filepath.Join(cwd, ".bifrost", "entries")
	outdir := filepath.Join(cwd, ".bifrost", "dist")
	ssrDir := filepath.Join(cwd, ".bifrost", "ssr")

	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		return fmt.Errorf("failed to create entries directory: %w", err)
	}

	absComponent := AbsoluteComponentPath(cwd, config.ComponentPath)
	if absComponent == "" {
		return fmt.Errorf("empty component path")
	}

	entryFile := filepath.Join(entryDir, entryName+adapter.EntryFileExtension())
	importPath, err := CalculateImportPath(entryFile, absComponent)
	if err != nil {
		return fmt.Errorf("failed to calculate import path: %w", err)
	}

	if err := WriteClientEntryFile(adapter, entryFile, importPath, config.Mode); err != nil {
		return fmt.Errorf("failed to write client entry file: %w", err)
	}

	if _, err := renderer.Build([]string{entryFile}, outdir, []string{entryName}); err != nil {
		return fmt.Errorf("failed to build client entry: %w", err)
	}

	// Dev always builds an SSR bundle so client-only routes can optionally render Head via SSR.
	if err := os.MkdirAll(ssrDir, 0o755); err != nil {
		return fmt.Errorf("failed to create SSR directory: %w", err)
	}

	ssrEntryName := entryName + "-ssr"
	ssrEntryFile := filepath.Join(entryDir, ssrEntryName+adapter.EntryFileExtension())
	ssrImportPath, err := CalculateImportPath(ssrEntryFile, absComponent)
	if err != nil {
		return fmt.Errorf("failed to calculate SSR import path: %w", err)
	}
	if err := WriteSSREntryFile(adapter, ssrEntryFile, ssrImportPath); err != nil {
		return fmt.Errorf("failed to write SSR entry file: %w", err)
	}
	if err := renderer.BuildSSR([]string{ssrEntryFile}, ssrDir); err != nil {
		return fmt.Errorf("failed to build SSR entry: %w", err)
	}

	return nil
}
