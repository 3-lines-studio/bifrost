package runtime

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func ExtractEmbeddedRuntime(assetsFS embed.FS) (string, func(), error) {
	runtimePath := filepath.Join(".bifrost", "runtime", "bifrost-renderer")
	if runtime.GOOS == "windows" {
		runtimePath += ".exe"
	}

	data, err := assetsFS.ReadFile(runtimePath)
	if err != nil {
		return "", nil, fmt.Errorf("embedded runtime not found: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "bifrost-runtime-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	executablePath := filepath.Join(tempDir, "bifrost-renderer")
	if runtime.GOOS == "windows" {
		executablePath += ".exe"
	}

	if err := os.WriteFile(executablePath, data, 0700); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", nil, fmt.Errorf("failed to write runtime executable: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(tempDir)
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
