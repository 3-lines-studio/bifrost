package runtime

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	esbuildadapter "github.com/3-lines-studio/bifrost/internal/adapters/esbuild"
	quickjsrenderer "github.com/3-lines-studio/bifrost/internal/adapters/quickjs"
	"github.com/3-lines-studio/bifrost/internal/core"
	"github.com/evanw/esbuild/pkg/api"
)

type rendererClient interface {
	Render(path string, props any) (core.RenderedPage, error)
	Build(entrypoints []string, outdir string, entryNames []string) (map[string]core.ClientBuildResult, error)
	BuildSSR(entrypoints []string, outdir string) error
	Stop() error
}

type Host struct {
	client     rendererClient
	assetsFS   embed.FS
	isDev      bool
	manifest   *core.Manifest
	ssrTempDir string
	ssrCleanup func()
	stopOnce   sync.Once
	stopErr    error
}

func NewHost(assetsFS embed.FS, mode core.Mode) (*Host, error) {
	r := &Host{
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

	return r.startInProcessRenderer(core.ModeProd, nil, ssrCleanup)
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
		setupErr := r.setupEmbeddedInProcessRuntime()
		if setupErr != nil {
			return nil, setupErr
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

func (r *Host) setupEmbeddedInProcessRuntime() error {
	ssrTempDir, ssrCleanup, err := extractSSRBundles(r.assetsFS, r.manifest)
	if err != nil {
		return fmt.Errorf("failed to extract SSR bundles: %w", err)
	}
	r.ssrTempDir = ssrTempDir
	r.ssrCleanup = ssrCleanup
	if err := r.startInProcessRenderer(core.ModeProd, nil, ssrCleanup); err != nil {
		return err
	}
	return r.primeWorkers()
}

// primeWorkers evaluates every SSR bundle on every worker so the first real
// request does not pay the cold-start eval (~160 ms per worker on large
// apps). Failures are warnings: the app still serves, the workers just stay
// cold.
func (r *Host) primeWorkers() error {
	primer, ok := r.client.(interface {
		Prime(bundlePaths []string) error
	})
	if !ok {
		return nil
	}
	seen := make(map[string]struct{})
	var paths []string
	for _, entry := range r.manifest.Entries {
		if entry.SSR == "" {
			continue
		}
		resolved := r.ResolveSSRBundlePath(entry.SSR)
		if bundlePath, _, found := strings.Cut(resolved, "#"); found {
			resolved = bundlePath
		}
		if resolved == "" {
			continue
		}
		if _, ok := seen[resolved]; ok {
			continue
		}
		seen[resolved] = struct{}{}
		paths = append(paths, resolved)
	}
	if len(paths) == 0 {
		return nil
	}
	// Non-fatal: the app still serves, the workers just stay cold and warm
	// on first use.
	_ = primer.Prime(paths)
	return nil
}

func (r *Host) initDevMode() (*Host, error) {
	builder := esbuildadapter.NewBuilder(core.ModeDev, esbuildadapter.WithSSRFormat(api.FormatESModule))
	if err := r.startInProcessRenderer(core.ModeDev, builder, nil); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Host) Client() rendererClient { return r.client }

func (r *Host) Manifest() *core.Manifest { return r.manifest }

func (r *Host) ResolveSSRBundlePath(manifestSSRPath string) string {
	if manifestSSRPath == "" {
		return ""
	}
	if r == nil || r.ssrTempDir == "" {
		return manifestSSRPath
	}
	return resolveStagedSSRBundlePath(r.ssrTempDir, manifestSSRPath)
}

func (r *Host) Stop() error {
	r.stopOnce.Do(func() {
		if r.client != nil {
			r.stopErr = r.client.Stop()
		}
		if r.ssrCleanup != nil {
			r.ssrCleanup()
		}
	})
	return r.stopErr
}

func copySSRBundlesFromDisk(exportDir string, manifest *core.Manifest) (string, func(), error) {
	read := func(manifestSSRPath string) ([]byte, error) {
		clean := strings.TrimPrefix(filepath.ToSlash(manifestSSRPath), "/")
		srcPath := filepath.Join(exportDir, filepath.FromSlash(clean))
		return os.ReadFile(srcPath)
	}
	return stageSSRBundles(read, manifest)
}

func (r *Host) startInProcessRenderer(mode core.Mode, builder inProcessBuilder, cleanup func()) error {
	client, err := quickjsrenderer.NewRenderer(mode, quickjsWorkers(), builder)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return fmt.Errorf("failed to start in-process runtime: %w", err)
	}
	r.client = client
	r.ssrCleanup = cleanup
	return nil
}

// inProcessBuilder matches the build methods shared by the in-process
// renderers. nil means builds are unavailable (production and export modes).
type inProcessBuilder interface {
	Build(entrypoints []string, outdir string, entryNames []string) (map[string]core.ClientBuildResult, error)
	BuildSSR(entrypoints []string, outdir string) error
}

func quickjsWorkers() int {
	value := os.Getenv("BIFROST_QUICKJS_WORKERS")
	if value == "" {
		return min(runtime.GOMAXPROCS(0), 8)
	}
	workers, err := strconv.Atoi(value)
	if err != nil || workers < 1 {
		return min(runtime.GOMAXPROCS(0), 8)
	}
	return workers
}
