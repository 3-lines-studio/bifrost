package process

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func ExtractEmbeddedRuntime(assetsFS embed.FS) (string, func(), error) {
	runtimePath := filepath.Join(".bifrost", "runtime", "bifrost-renderer")
	if runtime.GOOS == "windows" {
		runtimePath += ".exe"
	}

	data, err := assetsFS.ReadFile(runtimePath)
	if err != nil {
		return "", nil, fmt.Errorf("embedded runtime not found at %s: %w", runtimePath, err)
	}

	tempDir, err := os.MkdirTemp("", "bifrost-runtime-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	executablePath := filepath.Join(tempDir, "bifrost-renderer")
	if runtime.GOOS == "windows" {
		executablePath += ".exe"
	}

	if err := os.WriteFile(executablePath, data, 0755); err != nil {
		os.RemoveAll(tempDir)
		return "", nil, fmt.Errorf("failed to write runtime executable: %w", err)
	}

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	return executablePath, cleanup, nil
}

func HasEmbeddedRuntime(assetsFS embed.FS) bool {
	runtimePath := filepath.Join(".bifrost", "runtime", "bifrost-renderer")
	if runtime.GOOS == "windows" {
		runtimePath += ".exe"
	}

	_, err := assetsFS.ReadFile(runtimePath)
	return err == nil
}

func ExtractSSRBundles(assetsFS embed.FS, manifest *core.Manifest) (string, func(), error) {
	tempDir, err := os.MkdirTemp("", "bifrost-ssr-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create SSR temp dir: %w", err)
	}

	for entryName, entry := range manifest.Entries {
		if entry.SSR == "" {
			continue
		}

		embedPath := ".bifrost" + entry.SSR

		data, err := assetsFS.ReadFile(embedPath)
		if err != nil {
			os.RemoveAll(tempDir)
			return "", nil, fmt.Errorf("failed to read SSR bundle %s: %w", embedPath, err)
		}

		destPath := filepath.Join(tempDir, entry.SSR)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			os.RemoveAll(tempDir)
			return "", nil, fmt.Errorf("failed to create SSR dest dir: %w", err)
		}

		if err := os.WriteFile(destPath, data, 0644); err != nil {
			os.RemoveAll(tempDir)
			return "", nil, fmt.Errorf("failed to write SSR bundle %s: %w", entryName, err)
		}
	}

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	return tempDir, cleanup, nil
}
