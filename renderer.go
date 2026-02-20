package bifrost

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/3-lines-studio/bifrost/internal/adapters/process"
	"github.com/3-lines-studio/bifrost/internal/core"
)

type renderer struct {
	client     *process.Renderer
	assetsFS   embed.FS
	isDev      bool
	manifest   *core.Manifest
	ssrTempDir string
	ssrCleanup func()
}

func newRenderer(assetsFS embed.FS, mode core.Mode) (*renderer, error) {
	r := &renderer{
		isDev:    mode == core.ModeDev,
		assetsFS: assetsFS,
	}

	switch mode {
	case core.ModeExport:
		return r.initExportMode()
	case core.ModeProd:
		return r.initProdMode()
	default:
		return r.initDevMode()
	}
}

func (r *renderer) initExportMode() (*renderer, error) {
	exportDir := os.Getenv("BIFROST_EXPORT_DIR")
	if exportDir == "" {
		exportDir = ".bifrost"
	}

	man, err := loadManifestFromDisk(exportDir)
	if err != nil {
		return nil, err
	}
	r.manifest = man

	if core.HasSSREntries(man) {
		if err := r.setupRuntimeForExport(exportDir); err != nil {
			return nil, err
		}
	}

	return r, nil
}

func loadManifestFromDisk(exportDir string) (*core.Manifest, error) {
	manifestPath := filepath.Join(exportDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("manifest.json not found at %s: %w", manifestPath, err)
	}
	return core.ParseManifest(data)
}

func (r *renderer) setupRuntimeForExport(exportDir string) error {
	ssrTempDir, ssrCleanup, err := copySSRBundlesFromDisk(exportDir, r.manifest)
	if err != nil {
		return fmt.Errorf("failed to copy SSR bundles: %w", err)
	}
	r.ssrTempDir = ssrTempDir
	r.ssrCleanup = ssrCleanup

	client, err := process.NewRenderer(core.ModeProd)
	if err != nil {
		ssrCleanup()
		return fmt.Errorf("failed to start bun runtime: %w", err)
	}
	r.client = client
	return nil
}

func (r *renderer) initProdMode() (*renderer, error) {
	if r.assetsFS == (embed.FS{}) {
		return nil, fmt.Errorf("embed.FS is required in production mode")
	}

	man, err := loadManifestFromEmbed(r.assetsFS)
	if err != nil {
		return nil, err
	}
	r.manifest = man

	if core.HasSSREntries(man) {
		if err := r.setupEmbeddedRuntime(); err != nil {
			return nil, err
		}
	}

	return r, nil
}

func loadManifestFromEmbed(assetsFS embed.FS) (*core.Manifest, error) {
	data, err := assetsFS.ReadFile(".bifrost/manifest.json")
	if err != nil {
		return nil, fmt.Errorf("manifest.json not found in embedded assets: %w", err)
	}
	return core.ParseManifest(data)
}

func (r *renderer) setupEmbeddedRuntime() error {
	if !process.HasEmbeddedRuntime(r.assetsFS) {
		return fmt.Errorf("embedded runtime not found: run 'bifrost-build' to generate production assets")
	}

	ssrTempDir, ssrCleanup, err := process.ExtractSSRBundles(r.assetsFS, r.manifest)
	if err != nil {
		return fmt.Errorf("failed to extract SSR bundles: %w", err)
	}
	r.ssrTempDir = ssrTempDir
	r.ssrCleanup = ssrCleanup

	executablePath, cleanup, err := process.ExtractEmbeddedRuntime(r.assetsFS)
	if err != nil {
		ssrCleanup()
		return fmt.Errorf("failed to extract embedded runtime: %w", err)
	}

	combinedCleanup := func() {
		cleanup()
		ssrCleanup()
	}

	client, err := process.NewRendererFromExecutable(executablePath, combinedCleanup)
	if err != nil {
		cleanup()
		ssrCleanup()
		return fmt.Errorf("failed to start embedded runtime: %w", err)
	}
	r.client = client
	return nil
}

func (r *renderer) initDevMode() (*renderer, error) {
	client, err := process.NewRenderer(core.ModeDev)
	if err != nil {
		return nil, err
	}
	r.client = client
	return r, nil
}

func (r *renderer) stop() error {
	if r.client != nil {
		return r.client.Stop()
	}
	return nil
}

func copySSRBundlesFromDisk(exportDir string, manifest *core.Manifest) (string, func(), error) {
	tempDir, err := os.MkdirTemp("", "bifrost-ssr-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create SSR temp dir: %w", err)
	}

	for entryName, entry := range manifest.Entries {
		if entry.SSR == "" {
			continue
		}

		srcPath := filepath.Join(exportDir, entry.SSR)

		data, err := os.ReadFile(srcPath)
		if err != nil {
			os.RemoveAll(tempDir)
			return "", nil, fmt.Errorf("failed to read SSR bundle %s: %w", srcPath, err)
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
