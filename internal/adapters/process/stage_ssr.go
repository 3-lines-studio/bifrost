package process

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/core"
)

// ReadSSRBundle reads one SSR bundle by manifest-relative path (e.g. "/ssr/foo-ssr.js").
type ReadSSRBundle func(manifestSSRPath string) ([]byte, error)

// StageSSRBundles copies all non-empty SSR paths from the manifest into a temp directory,
// preserving path segments (e.g. /ssr/x.js -> temp/ssr/x.js). Used for both embedded
// assets and on-disk export layouts.
func StageSSRBundles(read ReadSSRBundle, manifest *core.Manifest) (tempDir string, cleanup func(), err error) {
	if manifest == nil {
		return "", nil, fmt.Errorf("manifest is nil")
	}
	tempDir, err = os.MkdirTemp("", "bifrost-ssr-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create SSR temp dir: %w", err)
	}

	cleanup = func() {
		_ = os.RemoveAll(tempDir)
	}

	for entryName, entry := range manifest.Entries {
		if entry.SSR == "" {
			continue
		}
		data, rerr := read(entry.SSR)
		if rerr != nil {
			cleanup()
			return "", nil, fmt.Errorf("failed to read SSR bundle %s: %w", entry.SSR, rerr)
		}
		destPath := ResolveStagedSSRBundlePath(tempDir, entry.SSR)
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("failed to create SSR dest dir: %w", err)
		}
		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("failed to write SSR bundle %s: %w", entryName, err)
		}
	}

	return tempDir, cleanup, nil
}

// ResolveStagedSSRBundlePath maps a manifest SSR path such as /ssr/page-ssr.js to the
// absolute path used inside an extracted SSR temp directory.
func ResolveStagedSSRBundlePath(tempDir string, manifestSSRPath string) string {
	clean := strings.TrimPrefix(filepath.ToSlash(manifestSSRPath), "/")
	return filepath.Join(tempDir, filepath.FromSlash(clean))
}
