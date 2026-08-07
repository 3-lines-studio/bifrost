package runtime

import (
	"embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/core"
)

// readSSRBundle reads one SSR bundle by manifest-relative path (e.g. "/ssr/foo-ssr.js").
type readSSRBundle func(manifestSSRPath string) ([]byte, error)

// stageSSRBundles copies all non-empty SSR paths from the manifest into a temp directory,
// preserving path segments (e.g. /ssr/x.js -> temp/ssr/x.js). Used for both embedded
// assets and on-disk export layouts.
func stageSSRBundles(read readSSRBundle, manifest *core.Manifest) (tempDir string, cleanup func(), err error) {
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

	staged := make(map[string]struct{})
	for entryName, entry := range manifest.Entries {
		if entry.SSR == "" {
			continue
		}
		clean, pathErr := cleanSSRBundlePath(entry.SSR)
		if pathErr != nil {
			cleanup()
			return "", nil, fmt.Errorf("invalid SSR bundle path for %s: %w", entryName, pathErr)
		}
		if _, ok := staged[clean]; ok {
			continue
		}
		staged[clean] = struct{}{}
		manifestBundlePath := "/" + clean
		data, rerr := read(manifestBundlePath)
		if rerr != nil {
			cleanup()
			return "", nil, fmt.Errorf("failed to read SSR bundle %s: %w", entry.SSR, rerr)
		}
		destPath := filepath.Join(tempDir, filepath.FromSlash(clean))
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

// extractSSRBundles stages the SSR bundles referenced by the manifest out of an
// embedded assets filesystem.
func extractSSRBundles(assetsFS embed.FS, manifest *core.Manifest) (string, func(), error) {
	read := func(manifestSSRPath string) ([]byte, error) {
		clean := strings.TrimPrefix(filepath.ToSlash(manifestSSRPath), "/")
		embedPath := path.Join(".bifrost", clean)
		return assetsFS.ReadFile(embedPath)
	}
	return stageSSRBundles(read, manifest)
}

// resolveStagedSSRBundlePath maps a manifest SSR path such as /ssr/page-ssr.js to the
// absolute path used inside an extracted SSR temp directory.
func resolveStagedSSRBundlePath(tempDir string, manifestSSRPath string) string {
	clean, err := cleanSSRBundlePath(manifestSSRPath)
	if err != nil {
		return ""
	}
	resolved := filepath.Join(tempDir, filepath.FromSlash(clean))
	if fragment := ssrBundleFragment(manifestSSRPath); fragment != "" {
		resolved += "#" + fragment
	}
	return resolved
}

func cleanSSRBundlePath(manifestSSRPath string) (string, error) {
	pathPart := strings.SplitN(manifestSSRPath, "#", 2)[0]
	clean := path.Clean(strings.TrimLeft(filepath.ToSlash(pathPart), "/"))
	if clean == "." || !strings.HasPrefix(clean, "ssr/") {
		return "", fmt.Errorf("path %q must stay under /ssr", manifestSSRPath)
	}
	return clean, nil
}

func ssrBundleFragment(manifestSSRPath string) string {
	parts := strings.SplitN(manifestSSRPath, "#", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}
