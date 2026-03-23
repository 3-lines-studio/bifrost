package process

import (
	"embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

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

	f, err := assetsFS.Open(runtimePath)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func ExtractSSRBundles(assetsFS embed.FS, manifest *core.Manifest) (string, func(), error) {
	read := func(manifestSSRPath string) ([]byte, error) {
		clean := strings.TrimPrefix(filepath.ToSlash(manifestSSRPath), "/")
		embedPath := path.Join(".bifrost", clean)
		return assetsFS.ReadFile(embedPath)
	}
	return StageSSRBundles(read, manifest)
}
