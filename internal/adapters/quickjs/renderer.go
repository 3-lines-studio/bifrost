package quickjs

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	qjs "github.com/buke/quickjs-go"

	"github.com/3-lines-studio/bifrost/internal/core"
)

//go:embed intl.js
var intlShim string

const (
	defaultExecTimeout = 30 * time.Second
	prebuiltIIFEMarker = "/* bifrost:sobek-iife */"
	prebuiltIIFEGlobal = "__BIFROST_SSR__"
)

// Builder builds client and SSR bundles. It is unused in production and
// export modes, where assets already exist.
type Builder interface {
	Build(entrypoints []string, outdir string, entryNames []string) (map[string]core.ClientBuildResult, error)
	BuildSSR(entrypoints []string, outdir string) error
}

// Renderer renders React SSR bundles with QuickJS. Each worker owns one
// runtime and context; workers are only ever used by one goroutine at a time.
type Renderer struct {
	mode        core.Mode
	builder     Builder
	workers     chan *worker
	execTimeout time.Duration
	stopped     atomic.Bool
}

type worker struct {
	rt          *qjs.Runtime
	ctx         *qjs.Context
	interrupted atomic.Bool
	modules     map[string]*loadedModule
	esmFiles    map[string]*esmFile
}

type esmFile struct {
	path    string
	version [sha256.Size]byte
}

type loadedModule struct {
	version [sha256.Size]byte
	render  *qjs.Value
}

func NewRenderer(mode core.Mode, workers int, builder Builder) (*Renderer, error) {
	if workers <= 0 {
		workers = min(runtime.GOMAXPROCS(0), 4)
	}
	r := &Renderer{
		mode:        mode,
		builder:     builder,
		workers:     make(chan *worker, workers),
		execTimeout: defaultExecTimeout,
	}
	for range workers {
		w, err := newWorker()
		if err != nil {
			for {
				select {
				case created := <-r.workers:
					created.close()
				default:
					return nil, fmt.Errorf("initialize QuickJS worker: %w", err)
				}
			}
		}
		r.workers <- w
	}
	return r, nil
}

func newWorker() (*worker, error) {
	rt := qjs.NewRuntime(qjs.WithModuleImport(true), qjs.WithOwnerGoroutineCheck(false))
	ctx := rt.NewContext()
	w := &worker{rt: rt, ctx: ctx, modules: make(map[string]*loadedModule), esmFiles: make(map[string]*esmFile)}
	rt.SetInterruptHandler(func() int {
		if w.interrupted.Load() {
			return 1
		}
		return 0
	})
	if err := installShims(ctx); err != nil {
		ctx.Close()
		rt.Close()
		return nil, err
	}
	return w, nil
}

func installShims(ctx *qjs.Context) error {
	if err := installConsole(ctx); err != nil {
		return err
	}
	shims := ctx.Eval(webAPIShims)
	if shims.IsException() {
		shims.Free()
		return fmt.Errorf("install web API shims: %w", ctx.Exception())
	}
	shims.Free()
	probe := ctx.Eval(`typeof Intl === "undefined"`)
	if probe.IsException() {
		return fmt.Errorf("probe Intl: %w", ctx.Exception())
	}
	intlMissing := probe.ToBool()
	probe.Free()
	if intlMissing {
		shim := ctx.Eval(intlShim)
		if shim.IsException() {
			return fmt.Errorf("install Intl shim: %w", ctx.Exception())
		}
		shim.Free()
	}
	return nil
}

func installConsole(ctx *qjs.Context) error {
	console := ctx.NewObject()
	write := func(file *os.File) func(*qjs.Context, *qjs.Value, []*qjs.Value) *qjs.Value {
		return func(_ *qjs.Context, _ *qjs.Value, args []*qjs.Value) *qjs.Value {
			parts := make([]string, len(args))
			for i, arg := range args {
				parts[i] = arg.String()
			}
			_, _ = fmt.Fprintln(file, strings.Join(parts, " "))
			return ctx.NewUndefined()
		}
	}
	console.Set("debug", ctx.Function(write(os.Stdout)))
	console.Set("info", ctx.Function(write(os.Stdout)))
	console.Set("log", ctx.Function(write(os.Stdout)))
	console.Set("warn", ctx.Function(write(os.Stderr)))
	console.Set("error", ctx.Function(write(os.Stderr)))
	ctx.Globals().Set("console", console)
	return nil
}

func (r *Renderer) Render(path string, props any) (core.RenderedPage, error) {
	return r.RenderContext(context.Background(), path, props)
}

func (r *Renderer) RenderContext(ctx context.Context, path string, props any) (core.RenderedPage, error) {
	if r == nil || r.stopped.Load() {
		return core.RenderedPage{}, fmt.Errorf("quickjs renderer is stopped")
	}
	if path == "" {
		return core.RenderedPage{}, fmt.Errorf("missing SSR bundle path")
	}
	if strings.Contains(path, "#") {
		return core.RenderedPage{}, fmt.Errorf("SSR registry bundles are not supported by the quickjs runtime")
	}

	ctx, cancel := context.WithTimeout(ctx, r.execTimeout)
	defer cancel()

	var w *worker
	select {
	case w = <-r.workers:
	case <-ctx.Done():
		return core.RenderedPage{}, ctx.Err()
	}
	defer func() {
		if r.stopped.Load() {
			w.close()
			return
		}
		r.workers <- w
	}()

	// Arm the interrupt before bundle evaluation so a module whose top-level
	// code never returns is also bounded by the deadline.
	finished := make(chan struct{})
	interruptFinished := make(chan struct{})
	go func() {
		defer close(interruptFinished)
		select {
		case <-ctx.Done():
			w.interrupted.Store(true)
		case <-finished:
		}
	}()
	disarm := func() {
		close(finished)
		<-interruptFinished
		w.interrupted.Store(false)
	}

	render, err := w.load(path, r.mode == core.ModeDev)
	if err != nil {
		disarm()
		return core.RenderedPage{}, structuredRenderError(err)
	}

	propsJSON, err := json.Marshal(props)
	if err != nil {
		disarm()
		return core.RenderedPage{}, err
	}
	propsValue := w.ctx.Eval("(" + string(propsJSON) + ")")
	if propsValue.IsException() {
		disarm()
		return core.RenderedPage{}, structuredRenderError(w.ctx.Exception())
	}
	defer propsValue.Free()

	result := w.ctx.Invoke(render, w.ctx.Globals(), propsValue)
	disarm()
	if result.IsException() {
		result.Free()
		if ctx.Err() != nil {
			_ = w.ctx.Exception()
			return core.RenderedPage{}, ctx.Err()
		}
		return core.RenderedPage{}, structuredRenderError(w.ctx.Exception())
	}
	defer result.Free()

	page, err := pageFromResult(ctx, w.ctx, result)
	if err != nil {
		return core.RenderedPage{}, structuredRenderError(err)
	}
	return page, nil
}

func pageFromResult(goCtx context.Context, ctx *qjs.Context, result *qjs.Value) (core.RenderedPage, error) {
	value := result
	if result.IsPromise() {
		// Await is only safe on settled promises; it never times out, so
		// pending promises are rejected before reaching it.
		switch result.PromiseState() {
		case qjs.PromiseFulfilled:
			awaited := ctx.Await(result)
			defer awaited.Free()
			if awaited.IsException() {
				return core.RenderedPage{}, ctx.Exception()
			}
			value = awaited
		case qjs.PromiseRejected:
			thrown := ctx.Await(result)
			thrown.Free()
			if goCtx.Err() != nil {
				_ = ctx.Exception()
				return core.RenderedPage{}, goCtx.Err()
			}
			return core.RenderedPage{}, ctx.Exception()
		default:
			return core.RenderedPage{}, fmt.Errorf("render returned a pending promise; asynchronous JavaScript SSR is unsupported")
		}
	}
	head := value.Get("head")
	defer head.Free()
	html := value.Get("html")
	defer html.Free()
	return core.RenderedPage{Head: head.String(), Body: html.String()}, nil
}

func (w *worker) load(path string, reload bool) (*qjs.Value, error) {
	if !reload {
		if module, ok := w.modules[path]; ok {
			return module.render, nil
		}
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read SSR bundle %q: %w", path, err)
	}
	version := sha256.Sum256(source)
	if module, ok := w.modules[path]; ok && module.version == version {
		return module.render, nil
	}

	render, err := w.evaluate(path, source, version)
	if err != nil {
		return nil, err
	}
	if previous, ok := w.modules[path]; ok {
		previous.render.Free()
	}
	w.modules[path] = &loadedModule{version: version, render: render}
	return render, nil
}

// mayContainModuleSyntax mirrors buke's module-source probe. Bundles with
// module syntax are loaded as native ESM; everything else evaluates as a
// script.
func mayContainModuleSyntax(source []byte) bool {
	return bytes.Contains(source, []byte("import")) ||
		bytes.Contains(source, []byte("export")) ||
		bytes.Contains(source, []byte("await"))
}

func (w *worker) evaluate(bundlePath string, source []byte, version [sha256.Size]byte) (*qjs.Value, error) {
	// Clear the previous bundle's global so a bundle that fails to define it
	// is not confused with an earlier one (dev reloads).
	w.ctx.Globals().Set(prebuiltIIFEGlobal, w.ctx.NewUndefined())

	if bytes.HasPrefix(bytes.TrimSpace(source), []byte(prebuiltIIFEMarker)) || !mayContainModuleSyntax(source) {
		// Prebuilt IIFE and plain scripts evaluate directly as scripts.
		value := w.ctx.Eval(string(source), qjs.EvalFileName(bundlePath))
		if value.IsException() {
			value.Free()
			return nil, fmt.Errorf("evaluate SSR bundle %q: %w", bundlePath, w.ctx.Exception())
		}
		value.Free()
	} else {
		// ESM bundles are imported from a version-unique sibling file, so
		// native module evaluation applies and dev reloads never resolve a
		// stale module cache entry.
		importPath, err := w.esmFile(bundlePath, source, version)
		if err != nil {
			return nil, fmt.Errorf("stage SSR bundle %q: %w", bundlePath, err)
		}
		imported := w.ctx.Eval(
			fmt.Sprintf("import * as ns from %q; globalThis.%s = ns;", importPath, prebuiltIIFEGlobal),
			qjs.EvalAwait(true),
		)
		if imported.IsException() {
			imported.Free()
			return nil, fmt.Errorf("evaluate SSR bundle %q: %w", bundlePath, w.ctx.Exception())
		}
		imported.Free()
	}

	global := w.ctx.Globals().Get(prebuiltIIFEGlobal)
	if global.IsUndefined() || global.IsNull() {
		global.Free()
		return nil, fmt.Errorf("SSR bundle did not define %s", prebuiltIIFEGlobal)
	}
	render := global.Get("render")
	global.Free()
	if !render.IsFunction() {
		render.Free()
		return nil, fmt.Errorf("SSR bundle did not export render")
	}
	return render, nil
}

// esmFile returns a version-unique sibling file containing the bundle source.
// The QuickJS module cache is keyed by the imported specifier, so a fresh
// path per version makes dev reloads safe. Versioned files are left in place;
// the staged SSR temp directory is removed by the host in production, and the
// dev .bifrost directory is a disposable build artifact.
func (w *worker) esmFile(bundlePath string, source []byte, version [sha256.Size]byte) (string, error) {
	if existing, ok := w.esmFiles[bundlePath]; ok && existing.version == version {
		return existing.path, nil
	}
	path := bundlePath + "." + hex.EncodeToString(version[:8]) + ".esm.js"
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(source); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil && !os.IsExist(err) {
		return "", err
	}
	w.esmFiles[bundlePath] = &esmFile{path: path, version: version}
	return path, nil
}

func (r *Renderer) Build(entrypoints []string, outdir string, entryNames []string) (map[string]core.ClientBuildResult, error) {
	if r.builder == nil {
		return nil, fmt.Errorf("quickjs renderer has no build adapter")
	}
	return r.builder.Build(entrypoints, outdir, entryNames)
}

func (r *Renderer) BuildSSR(entrypoints []string, outdir string) error {
	if r.builder == nil {
		return fmt.Errorf("quickjs renderer has no build adapter")
	}
	return r.builder.BuildSSR(entrypoints, outdir)
}

func (r *Renderer) Stop() error {
	if r == nil || r.stopped.Swap(true) {
		return nil
	}
	for {
		select {
		case w := <-r.workers:
			w.close()
		default:
			if stopper, ok := r.builder.(interface{ Stop() error }); ok {
				return stopper.Stop()
			}
			return nil
		}
	}
}

func (w *worker) close() {
	for _, module := range w.modules {
		module.render.Free()
	}
	w.ctx.Close()
	w.rt.Close()
}

func structuredRenderError(err error) error {
	if err == nil {
		return nil
	}

	message := err.Error()
	stack := ""
	var jsErr *qjs.Error
	if errors.As(err, &jsErr) {
		message = jsErr.Message
		stack = jsErr.Stack
	}
	message = strings.TrimPrefix(message, "Error: ")
	if !strings.HasPrefix(message, "Failed to import component:") {
		message = "Failed to import component: " + message
	}
	return &core.StructuredError{
		ErrorType: "Render Error",
		Message:   message,
		Stack:     stack,
	}
}

const webAPIShims = `
globalThis.queueMicrotask = (callback) => callback();
globalThis.MessageChannel = class MessageChannel {
	constructor() {
		this.port1 = { onmessage: null };
		this.port2 = {
			postMessage: (data) => {
				if (typeof this.port1.onmessage === "function") {
					this.port1.onmessage({ data });
				}
			},
		};
	}
};
globalThis.TextEncoder = class TextEncoder {
	encode(str) {
		const bytes = [];
		for (let i = 0; i < str.length; i++) {
			let cp = str.codePointAt(i);
			if (cp > 0xffff) i++;
			if (cp < 0x80) bytes.push(cp);
			else if (cp < 0x800) bytes.push(0xc0 | (cp >> 6), 0x80 | (cp & 0x3f));
			else if (cp < 0x10000) bytes.push(0xe0 | (cp >> 12), 0x80 | ((cp >> 6) & 0x3f), 0x80 | (cp & 0x3f));
			else bytes.push(0xf0 | (cp >> 18), 0x80 | ((cp >> 12) & 0x3f), 0x80 | ((cp >> 6) & 0x3f), 0x80 | (cp & 0x3f));
		}
		return Uint8Array.from(bytes);
	}
};
`
