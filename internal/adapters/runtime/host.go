package runtime

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/adapters/framework"
	"github.com/3-lines-studio/bifrost/internal/adapters/process"
	"github.com/3-lines-studio/bifrost/internal/core"
)

type Host struct {
	client     *process.Renderer
	assetsFS   embed.FS
	isDev      bool
	manifest   *core.Manifest
	ssrTempDir string
	ssrCleanup func()
	adapter    core.FrameworkAdapter
}

func NewHost(assetsFS embed.FS, mode core.Mode, adapter core.FrameworkAdapter) (*Host, error) {
	if adapter == nil {
		adapter = framework.NewReactAdapter()
	}

	r := &Host{
		isDev:    mode == core.ModeDev,
		assetsFS: assetsFS,
		adapter:  adapter,
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

func (r *Host) initExportMode() (*Host, error) {
	exportDir := os.Getenv("BIFROST_EXPORT_DIR")
	if exportDir == "" {
		exportDir = ".bifrost"
	}

	man, err := loadManifestFromDisk(exportDir)
	if err != nil {
		return nil, err
	}
	r.manifest = man

	if core.HasSSRBundles(man) {
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

func (r *Host) setupRuntimeForExport(exportDir string) error {
	ssrTempDir, ssrCleanup, err := copySSRBundlesFromDisk(exportDir, r.manifest)
	if err != nil {
		return fmt.Errorf("failed to copy SSR bundles: %w", err)
	}
	r.ssrTempDir = ssrTempDir
	r.ssrCleanup = ssrCleanup

	client, err := process.NewRenderer(core.ModeProd, r.adapter.ProdRendererSource())
	if err != nil {
		ssrCleanup()
		return fmt.Errorf("failed to start bun runtime: %w", err)
	}
	r.client = client
	return nil
}

func (r *Host) initProdMode() (*Host, error) {
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

func (r *Host) setupEmbeddedRuntime() error {
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

func (r *Host) initDevMode() (*Host, error) {
	client, err := process.NewRenderer(core.ModeDev, r.adapter.DevRendererSource())
	if err != nil {
		return nil, err
	}
	r.client = client
	return r, nil
}

func (h *Host) Client() *process.Renderer { return h.client }

func (h *Host) Manifest() *core.Manifest { return h.manifest }

func (h *Host) SSRTempDir() string { return h.ssrTempDir }

func (h *Host) IsDev() bool { return h.isDev }

func (h *Host) Stop() error {
	if h.client != nil {
		return h.client.Stop()
	}
	return nil
}

func copySSRBundlesFromDisk(exportDir string, manifest *core.Manifest) (string, func(), error) {
	read := func(manifestSSRPath string) ([]byte, error) {
		clean := strings.TrimPrefix(filepath.ToSlash(manifestSSRPath), "/")
		srcPath := filepath.Join(exportDir, filepath.FromSlash(clean))
		return os.ReadFile(srcPath)
	}
	return process.StageSSRBundles(read, manifest)
}
