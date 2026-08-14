//go:build !windows

package bifrost

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/3-lines-studio/bifrost/internal/dochtml"
	"github.com/3-lines-studio/bifrost/internal/protocol"
	"github.com/3-lines-studio/bifrost/internal/renderproc"
)

var ErrRendererBusy = errors.New("bifrost: renderer is busy")

func (a *App) initializeProduction(config Config) error {
	manifestData, err := fs.ReadFile(config.Assets, "manifest.json")
	if err != nil {
		return fmt.Errorf("bifrost: read manifest: %w", err)
	}
	wireManifest, err := parseManifest(manifestData)
	if err != nil {
		return err
	}
	manifest, err := validateManifest(config.Assets, a.spec, a.specHash, wireManifest)
	if err != nil {
		return err
	}

	var render renderer
	concurrency := config.RenderConcurrency
	if concurrency < 0 {
		return errors.New("bifrost: RenderConcurrency must not be negative")
	}
	if concurrency == 0 {
		concurrency = 1
	}
	queue := config.RenderQueue
	if queue <= 0 {
		queue = 64
	}
	if devDir := os.Getenv("BIFROST_DEV_DIR"); devDir != "" {
		port, parseErr := strconv.Atoi(os.Getenv("BIFROST_VITE_PORT"))
		if parseErr != nil || port < 1 || port > 65535 {
			return errors.New("bifrost: invalid BIFROST_VITE_PORT")
		}
		render, err = newDevelopmentRenderer(a.sourceRoot, devDir, port, concurrency, queue, a.logger, a.hooks.queueHooks)
	} else if wireManifest.Runtime != nil {
		render, err = newProductionRenderer(config.Assets, manifest, concurrency, queue, a.logger, a.hooks.queueHooks)
	}
	if err != nil {
		return err
	}

	compiled, err := compileRuntime(a, config.Assets, manifest, render)
	if err != nil {
		if render != nil {
			_ = render.Close(context.Background())
		}
		return err
	}
	a.runtime = compiled
	if err := a.Register(http.NewServeMux()); err != nil {
		a.runtime = nil
		if render != nil {
			_ = render.Close(context.Background())
		}
		return err
	}
	return nil
}

// Register installs Bifrost pages and assets into mux.
func (a *App) Register(mux *http.ServeMux) (err error) {
	if mux == nil {
		return errors.New("bifrost: nil ServeMux")
	}
	if a == nil || a.runtime == nil {
		return errors.New("bifrost: app has no compiled runtime")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("bifrost: register handlers: %v", recovered)
		}
	}()
	patterns := make([]string, 0, len(a.runtime.handlers))
	for pattern := range a.runtime.handlers {
		patterns = append(patterns, pattern)
	}
	slices.Sort(patterns)
	for _, pattern := range patterns {
		mux.Handle("GET "+pattern, a.runtime.handlers[pattern])
	}
	mux.HandleFunc("GET "+dochtml.AssetPrefix+"build-id", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(a.runtime.manifest.manifest.BuildID))
	})
	mux.Handle("GET "+dochtml.AssetPrefix+"{path...}", &assetHandler{assets: a.runtime.assets, files: a.runtime.files, headers: a.hooks.assetHeaders})
	publicURLs := make([]string, 0, len(a.runtime.public))
	for publicURL := range a.runtime.public {
		publicURLs = append(publicURLs, publicURL)
	}
	slices.Sort(publicURLs)
	for _, publicURL := range publicURLs {
		mux.Handle("GET "+publicURL, &publicAssetHandler{assets: a.runtime.assets, file: a.runtime.public[publicURL], headers: a.hooks.assetHeaders})
	}
	return nil
}

// Handler returns a standalone Bifrost handler. A setup failure produces a
// deterministic 503 response rather than a partially configured route table.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	if err := a.Register(mux); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		})
	}
	return a.ResolveMarkdown(mux)
}

// Ready checks whether every renderer process can accept work. Applications
// with only Static or Client routes are ready when their runtime is compiled.
func (a *App) Ready(ctx context.Context) error {
	if a == nil || a.runtime == nil {
		return errors.New("bifrost: app has no compiled runtime")
	}
	if a.runtime.renderer == nil {
		return nil
	}
	ready, ok := a.runtime.renderer.(interface{ Ready(context.Context) error })
	if !ok {
		return errors.New("bifrost: renderer does not report readiness")
	}
	return ready.Ready(ctx)
}

// Close drains renderer work until ctx ends, then stops child processes.
func (a *App) Close(ctx context.Context) error {
	if a == nil || a.runtime == nil || a.runtime.renderer == nil {
		return nil
	}
	return a.runtime.renderer.Close(ctx)
}

type rendererWorker struct {
	mu                 sync.RWMutex
	process            *renderproc.Process
	restartTimes       []time.Time
	restartLimitLogged bool
}

func (w *rendererWorker) current() *renderproc.Process {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.process
}

func (w *rendererWorker) replace(process *renderproc.Process) {
	w.mu.Lock()
	w.process = process
	w.mu.Unlock()
}

type productionRenderer struct {
	assets      fs.FS
	manifest    *compiledManifest
	root        string
	executable  string
	args        []string
	environment []string
	workDir     string
	cleanupRoot string
	admission   chan struct{}
	idle        chan *rendererWorker
	workers     []*rendererWorker

	lifecycleMu sync.Mutex
	closing     bool
	active      sync.WaitGroup

	closeOnce  sync.Once
	logger     *slog.Logger
	queueHooks []QueueHook
}

func newProductionRenderer(assets fs.FS, manifest *compiledManifest, concurrency, queue int, logger *slog.Logger, queueHooks []QueueHook) (*productionRenderer, error) {
	root, err := os.MkdirTemp("", "bifrost-runtime-")
	if err != nil {
		return nil, fmt.Errorf("bifrost: create runtime directory: %w", err)
	}
	cleanup := func(err error) (*productionRenderer, error) {
		_ = os.RemoveAll(root)
		return nil, err
	}
	if manifest.manifest.Runtime == nil {
		return cleanup(errors.New("bifrost: manifest has no renderer runtime"))
	}
	runtimePath := manifest.manifest.Runtime.Path
	if manifest.manifest.RuntimeCompression == "gzip" {
		runtimePath = strings.TrimSuffix(runtimePath, ".gz")
		if err := extractGzipArtifact(assets, root, *manifest.manifest.Runtime, runtimePath, 0o700); err != nil {
			return cleanup(err)
		}
	} else if err := extractArtifact(assets, root, *manifest.manifest.Runtime, 0o700); err != nil {
		return cleanup(err)
	}
	extracted := make(map[string]struct{})
	for _, view := range manifest.views {
		if view.Server == nil {
			continue
		}
		serverFiles := append([]protocol.FileRef{view.Server.Entry}, view.Server.Imports...)
		for _, file := range serverFiles {
			if _, exists := extracted[file.Path]; exists {
				continue
			}
			if err := extractArtifact(assets, root, file, 0o600); err != nil {
				return cleanup(err)
			}
			extracted[file.Path] = struct{}{}
		}
	}
	executable := filepath.Join(root, filepath.FromSlash(runtimePath))
	renderer := &productionRenderer{
		assets:      assets,
		manifest:    manifest,
		root:        root,
		executable:  executable,
		workDir:     root,
		cleanupRoot: root,
		admission:   make(chan struct{}, concurrency+queue),
		idle:        make(chan *rendererWorker, concurrency),
		workers:     make([]*rendererWorker, 0, concurrency),
		logger:      logger,
		queueHooks:  slices.Clone(queueHooks),
	}
	for index := range concurrency {
		process, startErr := renderproc.Start(executable, root)
		if startErr != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = renderer.closeWorkers(ctx)
			cancel()
			return cleanup(fmt.Errorf("bifrost: start renderer worker %d: %w", index+1, startErr))
		}
		worker := &rendererWorker{process: process}
		renderer.workers = append(renderer.workers, worker)
		renderer.idle <- worker
	}
	return renderer, nil
}

func newDevelopmentRenderer(sourceRoot, devDir string, port, _ int, queue int, logger *slog.Logger, queueHooks []QueueHook) (*productionRenderer, error) {
	bun, err := exec.LookPath("bun")
	if err != nil {
		return nil, errors.New("bifrost: Bun is required for development")
	}
	script := filepath.Join(devDir, "entries", "vite-dev.ts")
	if _, err := os.Stat(script); err != nil {
		return nil, fmt.Errorf("bifrost: development Vite bridge: %w", err)
	}
	environment := []string{
		"BIFROST_VITE_ROOT=" + sourceRoot,
		fmt.Sprintf("BIFROST_VITE_PORT=%d", port),
	}
	process, err := renderproc.StartCommand(bun, []string{"run", script}, sourceRoot, environment...)
	if err != nil {
		return nil, fmt.Errorf("bifrost: start Vite development bridge: %w", err)
	}
	worker := &rendererWorker{process: process}
	idle := make(chan *rendererWorker, 1)
	idle <- worker
	return &productionRenderer{
		executable:  bun,
		args:        []string{"run", script},
		environment: environment,
		workDir:     sourceRoot,
		admission:   make(chan struct{}, 1+queue),
		idle:        idle,
		workers:     []*rendererWorker{worker},
		logger:      logger,
		queueHooks:  slices.Clone(queueHooks),
	}, nil
}

func extractGzipArtifact(assets fs.FS, root string, ref protocol.FileRef, destinationPath string, mode fs.FileMode) error {
	source, err := assets.Open(ref.Path)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	reader, err := gzip.NewReader(source)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	return extractReader(root, destinationPath, reader, mode)
}

func extractArtifact(assets fs.FS, root string, ref protocol.FileRef, mode fs.FileMode) error {
	source, err := assets.Open(ref.Path)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	return extractReader(root, ref.Path, source, mode)
}

const maxExtractedArtifactBytes = 256 << 20

func extractReader(root, artifactPath string, source io.Reader, mode fs.FileMode) error {
	destination := filepath.Join(root, filepath.FromSlash(artifactPath))
	rel, err := filepath.Rel(root, destination)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("bifrost: artifact %q escapes extraction root", artifactPath)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	temporary := destination + ".tmp"
	output, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(output, io.LimitReader(source, maxExtractedArtifactBytes+1))
	if copyErr == nil && written > maxExtractedArtifactBytes {
		copyErr = fmt.Errorf("bifrost: extracted artifact %q exceeds %d bytes", artifactPath, maxExtractedArtifactBytes)
	}
	closeErr := output.Close()
	if copyErr != nil {
		_ = os.Remove(temporary)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(temporary)
		return closeErr
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func (r *productionRenderer) Render(ctx context.Context, request renderRequest, sink renderSink) error {
	r.lifecycleMu.Lock()
	if r.closing {
		r.lifecycleMu.Unlock()
		return ErrRendererBusy
	}
	r.active.Add(1)
	r.lifecycleMu.Unlock()
	defer r.active.Done()

	select {
	case r.admission <- struct{}{}:
		defer func() { <-r.admission }()
	default:
		r.observeQueue(ctx, QueueEvent{Pattern: request.Pattern, Err: ErrRendererBusy})
		return ErrRendererBusy
	}
	started := time.Now()
	var worker *rendererWorker
	select {
	case worker = <-r.idle:
		defer func() { r.idle <- worker }()
		r.observeQueue(ctx, QueueEvent{Pattern: request.Pattern, Wait: time.Since(started)})
	case <-ctx.Done():
		err := ctx.Err()
		r.observeQueue(ctx, QueueEvent{Pattern: request.Pattern, Wait: time.Since(started), Err: err})
		return err
	}

	process := worker.current()
	if process == nil {
		return errors.New("bifrost: renderer worker has no process")
	}
	entry := filepath.Join(r.root, filepath.FromSlash(request.Entry))
	err := process.Render(ctx, entry, request.Props, sink)
	var unavailable *renderproc.UnavailableError
	if errors.As(err, &unavailable) {
		r.restart(worker, process)
	}
	return err
}

func (r *productionRenderer) observeQueue(ctx context.Context, event QueueEvent) {
	for _, hook := range r.queueHooks {
		hook(ctx, event)
	}
}

func (r *productionRenderer) restart(worker *rendererWorker, failed *renderproc.Process) {
	if worker.current() != failed || !worker.allowRestart(time.Now(), r.logger) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_ = failed.Close(ctx)
	cancel()
	process, err := renderproc.StartCommand(r.executable, r.args, r.workDir, r.environment...)
	if err != nil {
		if r.logger != nil {
			r.logger.Error("bifrost renderer restart failed", "error", err)
		}
		return
	}
	worker.replace(process)
}

func (w *rendererWorker) allowRestart(now time.Time, logger *slog.Logger) bool {
	cutoff := now.Add(-time.Minute)
	kept := w.restartTimes[:0]
	for _, attempt := range w.restartTimes {
		if attempt.After(cutoff) {
			kept = append(kept, attempt)
		}
	}
	w.restartTimes = kept
	if len(w.restartTimes) >= 5 {
		if !w.restartLimitLogged && logger != nil {
			logger.Error("bifrost renderer restart limit reached", "attempts", len(w.restartTimes), "window", time.Minute)
		}
		w.restartLimitLogged = true
		return false
	}
	w.restartLimitLogged = false
	w.restartTimes = append(w.restartTimes, now)
	return true
}

func (r *productionRenderer) Ready(ctx context.Context) error {
	r.lifecycleMu.Lock()
	closing := r.closing
	r.lifecycleMu.Unlock()
	if closing {
		return errors.New("bifrost: renderer is closing")
	}
	if len(r.workers) == 0 {
		return errors.New("bifrost: renderer has no workers")
	}
	for index, worker := range r.workers {
		process := worker.current()
		if process == nil {
			return fmt.Errorf("bifrost: renderer worker %d has no process", index+1)
		}
		if err := process.Healthy(ctx); err != nil {
			return fmt.Errorf("bifrost: renderer worker %d is not ready: %w", index+1, err)
		}
	}
	return nil
}

func (r *productionRenderer) closeWorkers(ctx context.Context) error {
	var result error
	for _, worker := range r.workers {
		if process := worker.current(); process != nil {
			if err := process.Close(ctx); result == nil {
				result = err
			}
		}
	}
	return result
}

func (r *productionRenderer) Close(ctx context.Context) error {
	var result error
	r.closeOnce.Do(func() {
		r.lifecycleMu.Lock()
		r.closing = true
		r.lifecycleMu.Unlock()

		drained := make(chan struct{})
		go func() {
			r.active.Wait()
			close(drained)
		}()
		select {
		case <-drained:
		case <-ctx.Done():
			result = ctx.Err()
		}

		if err := r.closeWorkers(ctx); result == nil {
			result = err
		}
		if r.cleanupRoot != "" {
			if err := os.RemoveAll(r.cleanupRoot); result == nil {
				result = err
			}
		}
	})
	return result
}
