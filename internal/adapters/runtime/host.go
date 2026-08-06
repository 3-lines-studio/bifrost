package runtime

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	esbuildadapter "github.com/3-lines-studio/bifrost/internal/adapters/esbuild"
	moderncrenderer "github.com/3-lines-studio/bifrost/internal/adapters/modernc"
	"github.com/3-lines-studio/bifrost/internal/adapters/process"
	quickjsrenderer "github.com/3-lines-studio/bifrost/internal/adapters/quickjs"
	"github.com/3-lines-studio/bifrost/internal/adapters/react"
	sobekrenderer "github.com/3-lines-studio/bifrost/internal/adapters/sobek"
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
	if man.Runtime == "" {
		legacyRuntime := filepath.Join(exportDir, "runtime", "bifrost-renderer")
		if runtime.GOOS == "windows" {
			legacyRuntime += ".exe"
		}
		if _, statErr := os.Stat(legacyRuntime); statErr == nil {
			man.Runtime = core.JSRuntimeBun
		}
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

	if r.useBun() {
		return r.startRendererFromSource(core.ModeProd, react.RuntimeSource(core.ModeProd), ssrCleanup)
	}
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
	if man.Runtime == "" && process.HasEmbeddedRuntime(r.assetsFS) {
		man.Runtime = core.JSRuntimeBun
	}
	r.manifest = man

	if core.HasSSREntries(man) {
		var setupErr error
		if r.useBun() {
			setupErr = r.setupEmbeddedRuntime()
		} else {
			setupErr = r.setupEmbeddedInProcessRuntime()
		}
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
	ssrTempDir, ssrCleanup, err := process.ExtractSSRBundles(r.assetsFS, r.manifest)
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

func (r *Host) setupEmbeddedRuntime() error {
	if !process.HasEmbeddedRuntime(r.assetsFS) {
		return fmt.Errorf("embedded runtime not found: run 'bifrost build' to generate production assets")
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

	return r.startRendererFromExecutable(executablePath, combineCleanup(cleanup, ssrCleanup))
}

func (r *Host) initDevMode() (*Host, error) {
	if r.useBun() {
		if err := r.startRendererFromSource(core.ModeDev, react.RuntimeSource(core.ModeDev), nil); err != nil {
			return nil, err
		}
		return r, nil
	}

	builder := esbuildadapter.NewBuilder(core.ModeDev)
	if r.selectedRuntime() == core.JSRuntimeQuickJS || r.selectedRuntime() == core.JSRuntimeModernc {
		builder = esbuildadapter.NewBuilder(core.ModeDev, esbuildadapter.WithSSRFormat(api.FormatESModule))
	}
	if err := r.startInProcessRenderer(core.ModeDev, builder, nil); err != nil {
		return nil, err
	}
	return r, nil
}

func (h *Host) Client() rendererClient { return h.client }

func (h *Host) Manifest() *core.Manifest { return h.manifest }

func (h *Host) ResolveSSRBundlePath(manifestSSRPath string) string {
	if manifestSSRPath == "" {
		return ""
	}
	if h == nil || h.ssrTempDir == "" {
		return manifestSSRPath
	}
	return process.ResolveStagedSSRBundlePath(h.ssrTempDir, manifestSSRPath)
}

func (h *Host) Stop() error {
	h.stopOnce.Do(func() {
		if h.client != nil {
			h.stopErr = h.client.Stop()
		}
		if h.ssrCleanup != nil {
			h.ssrCleanup()
		}
	})
	return h.stopErr
}

func copySSRBundlesFromDisk(exportDir string, manifest *core.Manifest) (string, func(), error) {
	read := func(manifestSSRPath string) ([]byte, error) {
		clean := strings.TrimPrefix(filepath.ToSlash(manifestSSRPath), "/")
		srcPath := filepath.Join(exportDir, filepath.FromSlash(clean))
		return os.ReadFile(srcPath)
	}
	return process.StageSSRBundles(read, manifest)
}

func (r *Host) startRendererFromSource(mode core.Mode, source string, cleanup func()) error {
	var extraEnv []string
	if r.ssrTempDir != "" {
		cleanupDirs, err := json.Marshal([]string{r.ssrTempDir})
		if err != nil {
			if cleanup != nil {
				cleanup()
			}
			return fmt.Errorf("failed to encode runtime cleanup paths: %w", err)
		}
		extraEnv = append(extraEnv, "BIFROST_CLEANUP_DIRS="+string(cleanupDirs))
	}
	client, err := process.NewRenderer(mode, source, extraEnv...)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return fmt.Errorf("failed to start bun runtime: %w", err)
	}
	r.client = client
	r.ssrCleanup = cleanup
	return nil
}

func (r *Host) startRendererFromExecutable(executablePath string, cleanup func()) error {
	cleanupDirs, err := json.Marshal([]string{filepath.Dir(executablePath), r.ssrTempDir})
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return fmt.Errorf("failed to encode runtime cleanup paths: %w", err)
	}
	client, err := process.NewRendererFromExecutable(
		executablePath,
		nil,
		"BIFROST_CLEANUP_DIRS="+string(cleanupDirs),
	)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return fmt.Errorf("failed to start embedded runtime: %w", err)
	}
	r.client = client
	r.ssrCleanup = cleanup
	return nil
}

func (r *Host) startInProcessRenderer(mode core.Mode, builder inProcessBuilder, cleanup func()) error {
	var client rendererClient
	var err error
	switch r.selectedRuntime() {
	case core.JSRuntimeQuickJS:
		client, err = quickjsrenderer.NewRenderer(mode, quickjsWorkers(), builder)
	case core.JSRuntimeModernc:
		client, err = moderncrenderer.NewRenderer(mode, moderncWorkers(), builder)
	default:
		client, err = sobekrenderer.NewRenderer(mode, sobekWorkers(), builder)
	}
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

// inProcessBuilder matches the build methods shared by the Sobek and QuickJS
// adapters. nil means builds are unavailable (production and export modes).
type inProcessBuilder interface {
	Build(entrypoints []string, outdir string, entryNames []string) (map[string]core.ClientBuildResult, error)
	BuildSSR(entrypoints []string, outdir string) error
}

func (r *Host) useBun() bool {
	return r.selectedRuntime() == core.JSRuntimeBun
}

func (r *Host) useSobek() bool {
	return r.selectedRuntime() == core.JSRuntimeSobek
}

func (r *Host) useQuickJS() bool {
	return r.selectedRuntime() == core.JSRuntimeQuickJS
}

func (r *Host) selectedRuntime() string {
	selected := os.Getenv("BIFROST_JS_RUNTIME")
	if strings.TrimSpace(selected) == "" && r.manifest != nil {
		selected = r.manifest.Runtime
	}
	return core.NormalizeJSRuntime(selected)
}

func moderncWorkers() int {
	value := os.Getenv("BIFROST_MODERNC_WORKERS")
	if value == "" {
		return min(runtime.GOMAXPROCS(0), 4)
	}
	workers, err := strconv.Atoi(value)
	if err != nil || workers < 1 {
		return min(runtime.GOMAXPROCS(0), 4)
	}
	return workers
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

func sobekWorkers() int {
	value := os.Getenv("BIFROST_SOBEK_WORKERS")
	if value == "" {
		return min(runtime.GOMAXPROCS(0), 4)
	}
	workers, err := strconv.Atoi(value)
	if err != nil || workers < 1 {
		return min(runtime.GOMAXPROCS(0), 4)
	}
	return workers
}

func combineCleanup(cleanups ...func()) func() {
	return func() {
		for _, cleanup := range cleanups {
			if cleanup != nil {
				cleanup()
			}
		}
	}
}
