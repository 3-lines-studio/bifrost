//go:build !windows

package bifrost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
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
	"syscall"
	"time"

	"github.com/3-lines-studio/bifrost/internal/dochtml"
	"github.com/3-lines-studio/bifrost/internal/protocol"
	"github.com/3-lines-studio/bifrost/internal/renderproc"
)

var ErrRendererBusy = errors.New("bifrost: renderer is busy")

const (
	runtimeRetention = 7 * 24 * time.Hour
	renderQueueWait  = 2 * time.Second
	renderRetryAfter = "1"
)

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
		concurrency = 2
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
		if os.Getenv("BIFROST_EXTERNAL_VITE") != "" {
			for _, route := range wireManifest.Routes {
				if route.Kind != "client" {
					return fmt.Errorf("bifrost: external Vite development requires Client routes; route %q is %s", route.Pattern, route.Kind)
				}
			}
		} else if socket := os.Getenv("BIFROST_VITE_SOCKET"); socket != "" {
			render, err = newAttachedDevelopmentRenderer(socket, queue, a.logger, a.hooks.queueHooks)
		} else {
			routesFile, routeErr := writeDevelopmentRoutesFile(a.spec.Routes)
			if routeErr != nil && a.logger != nil {
				a.logger.Warn("bifrost: development route metadata unavailable", "error", routeErr)
			}
			render, err = newDevelopmentRenderer(a.sourceRoot, devDir, port, routesFile, concurrency, queue, a.logger, a.hooks.queueHooks)
		}
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
	mux.HandleFunc("GET "+dochtml.AssetPrefix+"ready", func(w http.ResponseWriter, request *http.Request) {
		if err := a.Ready(request.Context()); err != nil {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET "+dochtml.AssetPrefix+"build-id", func(w http.ResponseWriter, request *http.Request) {
		id := a.runtime.manifest.manifest.BuildID
		if request.URL.Query().Get("wait") == id && a.runtime.devProxy != nil {
			select {
			case <-request.Context().Done():
				return
			case <-time.After(30 * time.Second):
			}
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(id))
	})
	if a.runtime.devProxy != nil {
		mux.Handle("GET "+strings.TrimSuffix(dochtml.DevPrefix, "/")+"/{path...}", a.runtime.devProxy)
	}
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

type renderProcess interface {
	Render(context.Context, string, json.RawMessage, renderproc.Sink) error
	Healthy(context.Context) error
	Recover()
	Close(context.Context) error
}

type rendererWorker struct {
	mu                 sync.RWMutex
	process            renderProcess
	restartTimes       []time.Time
	restartLimitLogged bool
}

func (w *rendererWorker) current() renderProcess {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.process
}

func (w *rendererWorker) replace(process renderProcess) {
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
	devRoutes   string
	start       func() (renderProcess, error)
	attach      bool
	runtimeLock *runtimeLock
	admission   chan struct{}
	idle        chan *rendererWorker
	workers     []*rendererWorker

	lifecycleMu sync.Mutex
	closing     bool
	active      sync.WaitGroup

	closeOnce  sync.Once
	logger     *slog.Logger
	queueHooks []QueueHook
	queueWait  time.Duration
}

type runtimeLock struct {
	file *os.File
}

func runtimeBase() string {
	cache, err := os.UserCacheDir()
	if err != nil {
		return os.TempDir()
	}
	base := filepath.Join(cache, "bifrost")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return os.TempDir()
	}
	return base
}

func runtimeDirectory(manifest *compiledManifest) (string, *runtimeLock, error) {
	base := runtimeBase()
	root := filepath.Join(base, "bifrost-runtime-"+manifest.manifest.BuildID)
	info, err := os.Lstat(root)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", nil, fmt.Errorf("bifrost: inspect runtime directory: %w", err)
	}
	if err == nil && !info.IsDir() {
		return "", nil, fmt.Errorf("bifrost: runtime directory %s is not a directory", root)
	}
	lock, err := newRuntimeLock(root)
	if err != nil {
		return "", nil, err
	}
	now := time.Now()
	_ = os.Chtimes(root, now, now)
	pruneRuntimeDirectories(base, root)
	return root, lock, nil
}

func newRuntimeLock(root string) (*runtimeLock, error) {
	file, err := os.OpenFile(filepath.Join(root, "lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if errors.Is(err, fs.ErrNotExist) {
		if mkdirErr := os.MkdirAll(root, 0o700); mkdirErr != nil {
			return nil, fmt.Errorf("bifrost: create runtime directory: %w", mkdirErr)
		}
		file, err = os.OpenFile(filepath.Join(root, "lock"), os.O_CREATE|os.O_RDWR, 0o600)
	}
	if err != nil {
		return nil, fmt.Errorf("bifrost: lock runtime directory: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_SH); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("bifrost: lock runtime directory: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("bifrost: create runtime directory: %w", err)
	}
	return &runtimeLock{file: file}, nil
}

func (l *runtimeLock) release() {
	if l == nil {
		return
	}
	_ = l.file.Close()
}

func pruneRuntimeDirectories(base, current string) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "bifrost-runtime-") {
			continue
		}
		root := filepath.Join(base, entry.Name())
		if root == current {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < runtimeRetention {
			continue
		}
		_ = removeRuntimeDirectory(root)
	}
}

func removeRuntimeDirectory(root string) error {
	file, err := os.OpenFile(filepath.Join(root, "lock"), os.O_RDWR, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return os.RemoveAll(root)
		}
		return err
	}
	defer func() { _ = file.Close() }()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil
		}
		return err
	}
	return os.RemoveAll(root)
}

func publishRuntimeScript(root string, ref protocol.FileRef, destination string) error {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ref.Path)))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(destination), ".bifrost-runtime-*")
	if err != nil {
		return err
	}
	temporary := temp.Name()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		_ = os.Remove(temporary)
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		_ = os.Remove(temporary)
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func hostBun() (string, error) {
	if value := os.Getenv("BIFROST_BUN"); value != "" {
		return value, nil
	}
	value, err := exec.LookPath("bun")
	if err != nil {
		return "", errors.New("bifrost: the renderer needs Bun on the host: install bun, set BIFROST_BUN, or build with --embed-bun")
	}
	return value, nil
}

func newProductionRenderer(assets fs.FS, manifest *compiledManifest, concurrency, queue int, logger *slog.Logger, queueHooks []QueueHook) (*productionRenderer, error) {
	root, lock, err := runtimeDirectory(manifest)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*productionRenderer, error) {
		lock.release()
		return nil, err
	}
	if manifest.manifest.Runtime == nil {
		return fail(errors.New("bifrost: manifest has no renderer runtime"))
	}
	if err := extractVerifiedArtifact(assets, root, *manifest.manifest.Runtime, 0o700); err != nil {
		return fail(err)
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
				return fail(err)
			}
			extracted[file.Path] = struct{}{}
		}
	}
	if socket := os.Getenv("BIFROST_RENDERER_SOCKET"); socket != "" {
		if concurrency > 1 {
			if logger != nil {
				logger.Warn("bifrost: shared renderer uses one worker", "requested", concurrency)
			}
			concurrency = 1
		}
		if scriptPath := os.Getenv("BIFROST_RUNTIME_SCRIPT"); scriptPath != "" && manifest.manifest.RuntimeKind == "bun" {
			if err := publishRuntimeScript(root, *manifest.manifest.Runtime, scriptPath); err != nil {
				return fail(err)
			}
		}
		renderer := &productionRenderer{
			assets:      assets,
			manifest:    manifest,
			root:        root,
			attach:      true,
			workDir:     root,
			runtimeLock: lock,
			admission:   make(chan struct{}, concurrency+queue),
			idle:        make(chan *rendererWorker, concurrency),
			workers:     make([]*rendererWorker, 0, concurrency),
			logger:      logger,
			queueHooks:  slices.Clone(queueHooks),
		}
		for index := range concurrency {
			process, attachErr := renderproc.Attach(socket)
			if attachErr != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				_ = renderer.closeWorkers(ctx)
				cancel()
				return fail(fmt.Errorf("bifrost: attach renderer worker %d: %w", index+1, attachErr))
			}
			worker := &rendererWorker{process: process}
			renderer.workers = append(renderer.workers, worker)
			renderer.idle <- worker
		}
		return renderer, nil
	}

	runtimePath := filepath.Join(root, filepath.FromSlash(manifest.manifest.Runtime.Path))
	executable := runtimePath
	var args []string
	if manifest.manifest.RuntimeKind == "bun" {
		executable, err = hostBun()
		if err != nil {
			return fail(err)
		}
		args = []string{runtimePath}
	}
	renderer := &productionRenderer{
		assets:      assets,
		manifest:    manifest,
		root:        root,
		executable:  executable,
		args:        args,
		workDir:     root,
		runtimeLock: lock,
		start: func() (renderProcess, error) {
			return renderproc.StartCommand(executable, args, root)
		},
		admission:  make(chan struct{}, concurrency+queue),
		idle:       make(chan *rendererWorker, concurrency),
		workers:    make([]*rendererWorker, 0, concurrency),
		logger:     logger,
		queueHooks: slices.Clone(queueHooks),
	}
	for index := range concurrency {
		process, startErr := renderproc.StartCommand(executable, args, root)
		if startErr != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = renderer.closeWorkers(ctx)
			cancel()
			return fail(fmt.Errorf("bifrost: start renderer worker %d: %w", index+1, startErr))
		}
		worker := &rendererWorker{process: process}
		renderer.workers = append(renderer.workers, worker)
		renderer.idle <- worker
	}
	return renderer, nil
}

// writeDevelopmentRoutesFile publishes the route table for the development
// bridge's virtual routes module.
func writeDevelopmentRoutesFile(routes []protocol.RouteSpec) (string, error) {
	if len(routes) == 0 {
		return "", nil
	}
	data, err := json.Marshal(routes)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", "bifrost-routes-*.json")
	if err != nil {
		return "", err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

func newDevelopmentRenderer(sourceRoot, devDir string, port int, routesFile string, _ int, queue int, logger *slog.Logger, queueHooks []QueueHook) (*productionRenderer, error) {
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
	if routesFile != "" {
		environment = append(environment, "BIFROST_ROUTES_FILE="+routesFile)
	}
	environment = append(environment, "BIFROST_DEV_ENTRIES="+filepath.Join(devDir, "entries"))
	bridge, err := renderproc.StartBridge(bun, []string{"run", script}, sourceRoot, environment...)
	if err != nil {
		return nil, fmt.Errorf("bifrost: start Vite development bridge: %w", err)
	}
	worker := &rendererWorker{process: bridge}
	idle := make(chan *rendererWorker, 1)
	idle <- worker
	return &productionRenderer{
		workDir:    sourceRoot,
		devRoutes:  routesFile,
		admission:  make(chan struct{}, 1+queue),
		idle:       idle,
		workers:    []*rendererWorker{worker},
		logger:     logger,
		queueHooks: slices.Clone(queueHooks),
	}, nil
}

// newAttachedDevelopmentRenderer connects to a Vite bridge that outlives this
// process and is owned by the bifrost dev command. Restart policy belongs to
// the owner.
func newAttachedDevelopmentRenderer(socket string, queue int, logger *slog.Logger, queueHooks []QueueHook) (*productionRenderer, error) {
	bridge, err := renderproc.AttachBridge(socket)
	if err != nil {
		return nil, fmt.Errorf("bifrost: attach Vite development bridge: %w", err)
	}
	worker := &rendererWorker{process: bridge}
	idle := make(chan *rendererWorker, 1)
	idle <- worker
	return &productionRenderer{
		attach:     true,
		admission:  make(chan struct{}, 1+queue),
		idle:       idle,
		workers:    []*rendererWorker{worker},
		logger:     logger,
		queueHooks: slices.Clone(queueHooks),
	}, nil
}

func extractArtifact(assets fs.FS, root string, ref protocol.FileRef, mode fs.FileMode) error {
	source, err := assets.Open(ref.Path)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	return extractReader(root, ref.Path, source, mode)
}

func extractVerifiedArtifact(assets fs.FS, root string, ref protocol.FileRef, mode fs.FileMode) error {
	source, err := assets.Open(ref.Path)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	return extractVerifiedReader(root, ref, source, mode)
}

const maxExtractedArtifactBytes = 256 << 20

func extractReader(root, artifactPath string, source io.Reader, mode fs.FileMode) error {
	return extractVerifiedReader(root, protocol.FileRef{Path: artifactPath}, source, mode)
}

func extractVerifiedReader(root string, ref protocol.FileRef, source io.Reader, mode fs.FileMode) error {
	artifactPath := ref.Path
	destination := filepath.Join(root, filepath.FromSlash(artifactPath))
	rel, err := filepath.Rel(root, destination)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("bifrost: artifact %q escapes extraction root", artifactPath)
	}
	if info, err := os.Stat(destination); err == nil && info.Mode().IsRegular() {
		if ref.Hash != "" && info.Size() != ref.Size {
			_ = os.Remove(destination)
		} else {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	output, err := os.CreateTemp(filepath.Dir(destination), filepath.Base(destination)+".*")
	if err != nil {
		return err
	}
	temporary := output.Name()
	if err := output.Chmod(mode); err != nil {
		_ = output.Close()
		_ = os.Remove(temporary)
		return err
	}
	var digest hash.Hash
	writer := io.Writer(output)
	if ref.Hash != "" {
		digest = sha256.New()
		writer = io.MultiWriter(output, digest)
	}
	written, copyErr := io.Copy(writer, io.LimitReader(source, maxExtractedArtifactBytes+1))
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
	if digest != nil {
		actualHash := hex.EncodeToString(digest.Sum(nil))
		if actualHash != ref.Hash {
			_ = os.Remove(temporary)
			return fmt.Errorf("bifrost: artifact %q hash %s does not match manifest hash %s", artifactPath, actualHash, ref.Hash)
		}
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

	wait := r.queueWait
	if wait <= 0 {
		wait = renderQueueWait
	}
	started := time.Now()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	busy := func() error {
		r.observeQueue(ctx, QueueEvent{Pattern: request.Pattern, Wait: time.Since(started), Err: ErrRendererBusy})
		return ErrRendererBusy
	}
	select {
	case r.admission <- struct{}{}:
		defer func() { <-r.admission }()
	case <-timer.C:
		return busy()
	case <-ctx.Done():
		err := ctx.Err()
		r.observeQueue(ctx, QueueEvent{Pattern: request.Pattern, Wait: time.Since(started), Err: err})
		return err
	}
	var worker *rendererWorker
	select {
	case worker = <-r.idle:
		defer func() { r.idle <- worker }()
		r.observeQueue(ctx, QueueEvent{Pattern: request.Pattern, Wait: time.Since(started)})
	case <-timer.C:
		return busy()
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
		if r.attach {
			process.Recover()
		} else {
			r.restart(worker, process)
		}
	}
	return err
}

func (r *productionRenderer) observeQueue(ctx context.Context, event QueueEvent) {
	for _, hook := range r.queueHooks {
		hook(ctx, event)
	}
}

func (r *productionRenderer) restart(worker *rendererWorker, failed renderProcess) {
	if worker.current() != failed || !worker.allowRestart(time.Now(), r.logger) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_ = failed.Close(ctx)
	cancel()
	process, err := r.start()
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
		if r.devRoutes != "" {
			_ = os.Remove(r.devRoutes)
		}
		if r.runtimeLock != nil {
			r.runtimeLock.release()
		}
	})
	return result
}
