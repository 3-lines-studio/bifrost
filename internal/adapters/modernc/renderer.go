package modernc

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
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	quickjs "modernc.org/quickjs"

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

// Renderer renders React SSR bundles with the pure-Go QuickJS port
// (modernc.org/quickjs). Each worker owns one VM; workers are only ever used
// by one goroutine at a time.
type Renderer struct {
	mode        core.Mode
	builder     Builder
	workers     chan *worker
	execTimeout time.Duration
	stopped     atomic.Bool
}

type worker struct {
	vm          *quickjs.VM
	global      quickjs.Value
	undefined   quickjs.Value
	isPromiseFn quickjs.Value
	ssrAtom     quickjs.Atom
	renderAtom  quickjs.Atom
	headAtom    quickjs.Atom
	htmlAtom    quickjs.Atom
	modules     map[string]*loadedModule
	esmFiles    map[string]*esmFile
	renderSeq   uint64
}

type loadedModule struct {
	version [sha256.Size]byte
	render  quickjs.Value
}

type esmFile struct {
	path    string
	version [sha256.Size]byte
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
					return nil, fmt.Errorf("initialize modernc worker: %w", err)
				}
			}
		}
		r.workers <- w
	}
	return r, nil
}

func newWorker() (*worker, error) {
	vm, err := quickjs.NewVM()
	if err != nil {
		return nil, err
	}
	_ = vm.SetEvalTimeout(defaultExecTimeout)
	vm.SetGCThreshold(uintptr(moderncGCThreshold()))
	vm.SetDefaultModuleLoader()

	w := &worker{
		vm:       vm,
		global:   vm.GlobalObject(),
		modules:  make(map[string]*loadedModule),
		esmFiles: make(map[string]*esmFile),
	}
	for _, atom := range []struct {
		name string
		dst  *quickjs.Atom
	}{
		{prebuiltIIFEGlobal, &w.ssrAtom},
		{"render", &w.renderAtom},
		{"head", &w.headAtom},
		{"html", &w.htmlAtom},
	} {
		created, err := vm.NewAtom(atom.name)
		if err != nil {
			w.close()
			return nil, err
		}
		*atom.dst = created
	}
	if w.undefined, err = vm.EvalValue("undefined", quickjs.EvalGlobal); err != nil {
		w.close()
		return nil, err
	}
	if w.isPromiseFn, err = vm.EvalValue("(v) => v instanceof Promise", quickjs.EvalGlobal); err != nil {
		w.close()
		return nil, err
	}
	if err := w.installShims(); err != nil {
		w.close()
		return nil, err
	}
	return w, nil
}

func (w *worker) installShims() error {
	if err := w.installConsole(); err != nil {
		return err
	}
	if _, err := w.vm.Eval(webAPIShims, quickjs.EvalGlobal); err != nil {
		return fmt.Errorf("install web API shims: %w", err)
	}
	probe, err := w.vm.Eval(`typeof Intl === "undefined"`, quickjs.EvalGlobal)
	if err != nil {
		return fmt.Errorf("probe Intl: %w", err)
	}
	if missing, ok := probe.(bool); ok && missing {
		if _, err := w.vm.Eval(intlShim, quickjs.EvalGlobal); err != nil {
			return fmt.Errorf("install Intl shim: %w", err)
		}
	}
	return nil
}

func (w *worker) installConsole() error {
	write := func(file *os.File) func([]any) (any, error) {
		return func(args []any) (any, error) {
			parts := make([]string, len(args))
			for i, arg := range args {
				parts[i] = fmt.Sprint(arg)
			}
			_, _ = fmt.Fprintln(file, strings.Join(parts, " "))
			return nil, nil
		}
	}
	if err := w.vm.RegisterHostFunc("__bifrostLog", write(os.Stdout)); err != nil {
		return fmt.Errorf("install console.log: %w", err)
	}
	if err := w.vm.RegisterHostFunc("__bifrostErr", write(os.Stderr)); err != nil {
		return fmt.Errorf("install console.error: %w", err)
	}
	if _, err := w.vm.Eval(consoleShim, quickjs.EvalGlobal); err != nil {
		return fmt.Errorf("install console: %w", err)
	}
	return nil
}

func (r *Renderer) Render(path string, props any) (core.RenderedPage, error) {
	return r.RenderContext(context.Background(), path, props)
}

func (r *Renderer) RenderContext(ctx context.Context, path string, props any) (core.RenderedPage, error) {
	if r == nil || r.stopped.Load() {
		return core.RenderedPage{}, fmt.Errorf("modernc renderer is stopped")
	}
	if path == "" {
		return core.RenderedPage{}, fmt.Errorf("missing SSR bundle path")
	}
	if strings.Contains(path, "#") {
		return core.RenderedPage{}, fmt.Errorf("SSR registry bundles are not supported by the modernc runtime")
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
			w.vm.Interrupt()
		case <-finished:
		}
	}()
	disarm := func() {
		close(finished)
		<-interruptFinished
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
	propsValue, err := w.vm.Eval("("+string(propsJSON)+")", quickjs.EvalGlobal)
	if err != nil {
		disarm()
		return core.RenderedPage{}, structuredRenderError(err)
	}

	result, err := render.CallValue(w.undefined, propsValue)
	disarm()
	if err != nil {
		if ctx.Err() != nil {
			return core.RenderedPage{}, ctx.Err()
		}
		return core.RenderedPage{}, structuredRenderError(err)
	}
	defer result.Free()

	page, err := w.pageFromResult(ctx, result)
	if err != nil {
		return core.RenderedPage{}, structuredRenderError(err)
	}
	return page, nil
}

func (w *worker) pageFromResult(ctx context.Context, result quickjs.Value) (core.RenderedPage, error) {
	w.renderSeq++
	seq := w.renderSeq
	promise, err := w.isPromiseFn.Call(w.undefined, result)
	if err != nil {
		return core.RenderedPage{}, err
	}
	if !promise.(bool) {
		return w.pageFromObject(result)
	}

	type outcome struct {
		page core.RenderedPage
		err  error
	}
	done := make(chan outcome, 1)
	chain, err := result.Then(
		func(v quickjs.Value) {
			if w.renderSeq != seq {
				return
			}
			page, err := w.pageFromObject(v)
			done <- outcome{page, err}
		},
		func(v quickjs.Value) {
			if w.renderSeq != seq {
				return
			}
			message, _ := v.Any()
			done <- outcome{core.RenderedPage{}, fmt.Errorf("render rejected: %v", message)}
		},
	)
	if err != nil {
		return core.RenderedPage{}, err
	}
	chain.Free()

	for {
		if _, err := w.vm.ExecutePendingJobs(); err != nil {
			return core.RenderedPage{}, err
		}
		select {
		case o := <-done:
			return o.page, o.err
		case <-ctx.Done():
			return core.RenderedPage{}, ctx.Err()
		default:
		}
		time.Sleep(time.Millisecond)
	}
}

func (w *worker) pageFromObject(obj quickjs.Value) (core.RenderedPage, error) {
	html, err := obj.GetPropertyValue(w.htmlAtom)
	if err != nil {
		return core.RenderedPage{}, err
	}
	defer html.Free()
	head, err := obj.GetPropertyValue(w.headAtom)
	if err != nil {
		return core.RenderedPage{}, err
	}
	defer head.Free()

	body, err := toString(html)
	if err != nil {
		return core.RenderedPage{}, err
	}
	headValue, err := toString(head)
	if err != nil {
		return core.RenderedPage{}, err
	}
	return core.RenderedPage{Head: headValue, Body: body}, nil
}

// toString converts a JS value to its string form, degrading non-strings
// like the other backends instead of panicking on a type assertion.
func toString(value quickjs.Value) (string, error) {
	anyValue, err := value.Any()
	if err != nil {
		return "", err
	}
	switch typed := anyValue.(type) {
	case string:
		return typed, nil
	case nil:
		return "", nil
	default:
		return fmt.Sprint(typed), nil
	}
}

func (w *worker) load(path string, reload bool) (quickjs.Value, error) {
	if !reload {
		if module, ok := w.modules[path]; ok {
			return module.render, nil
		}
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return quickjs.Value{}, fmt.Errorf("read SSR bundle %q: %w", path, err)
	}
	version := sha256.Sum256(source)
	if module, ok := w.modules[path]; ok && module.version == version {
		return module.render, nil
	}

	render, err := w.evaluate(path, source, version)
	if err != nil {
		return quickjs.Value{}, err
	}
	if previous, ok := w.modules[path]; ok {
		previous.render.Free()
	}
	w.modules[path] = &loadedModule{version: version, render: render}
	return render, nil
}

// mayContainModuleSyntax mirrors the module-source probes of the QuickJS
// bindings. Bundles with module syntax are loaded as native ESM; everything
// else evaluates as a script.
func mayContainModuleSyntax(source []byte) bool {
	return bytes.Contains(source, []byte("import")) ||
		bytes.Contains(source, []byte("export")) ||
		bytes.Contains(source, []byte("await"))
}

func (w *worker) evaluate(bundlePath string, source []byte, version [sha256.Size]byte) (quickjs.Value, error) {
	// Clear the previous bundle's global so a bundle that fails to define it
	// is not confused with an earlier one (dev reloads).
	if err := w.global.SetPropertyValue(w.ssrAtom, w.undefined); err != nil {
		return quickjs.Value{}, err
	}

	if bytes.HasPrefix(bytes.TrimSpace(source), []byte(prebuiltIIFEMarker)) || !mayContainModuleSyntax(source) {
		if _, err := w.vm.Eval(string(source), quickjs.EvalGlobal); err != nil {
			return quickjs.Value{}, fmt.Errorf("evaluate SSR bundle %q: %w", bundlePath, err)
		}
	} else {
		// ESM bundles are imported from a version-unique sibling file, so
		// native module evaluation applies and dev reloads never resolve a
		// stale module cache entry.
		importPath, err := w.esmFile(bundlePath, source, version)
		if err != nil {
			return quickjs.Value{}, fmt.Errorf("stage SSR bundle %q: %w", bundlePath, err)
		}
		if _, err := w.vm.Eval(
			fmt.Sprintf("import * as ns from %q; globalThis.%s = ns;", importPath, prebuiltIIFEGlobal),
			quickjs.EvalModule,
		); err != nil {
			return quickjs.Value{}, fmt.Errorf("evaluate SSR bundle %q: %w", bundlePath, err)
		}
		if _, err := w.vm.ExecutePendingJobs(); err != nil {
			return quickjs.Value{}, fmt.Errorf("execute SSR bundle %q: %w", bundlePath, err)
		}
	}

	state, err := w.vm.Eval(
		"(typeof globalThis."+prebuiltIIFEGlobal+" === 'undefined' || globalThis."+prebuiltIIFEGlobal+" === null)"+
			" ? 'missing'"+
			" : (typeof globalThis."+prebuiltIIFEGlobal+".render === 'function' ? 'ok' : 'no-render')",
		quickjs.EvalGlobal,
	)
	if err != nil {
		return quickjs.Value{}, err
	}
	switch state {
	case "missing":
		return quickjs.Value{}, fmt.Errorf("SSR bundle did not define %s", prebuiltIIFEGlobal)
	case "no-render":
		return quickjs.Value{}, fmt.Errorf("SSR bundle did not export render")
	}
	render, err := w.vm.EvalValue("globalThis."+prebuiltIIFEGlobal+".render", quickjs.EvalGlobal)
	if err != nil {
		return quickjs.Value{}, err
	}
	return render, nil
}

// moderncGCThreshold returns the per-runtime automatic-GC threshold in
// bytes. The classic quickjs port leaves garbage mostly uncollected under
// load (measured +6 MB/s at 300 renders/s), so default to 16 MiB like the
// quickjs-ng backend; BIFROST_MODERNC_GC_THRESHOLD overrides it.
func moderncGCThreshold() uintptr {
	value := os.Getenv("BIFROST_MODERNC_GC_THRESHOLD")
	if value == "" {
		return 16 << 20
	}
	threshold, err := strconv.ParseUint(value, 10, 64)
	if err != nil || threshold == 0 {
		return 16 << 20
	}
	return uintptr(threshold)
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

// Prime evaluates each SSR bundle on every worker so the first real request
// does not pay the cold-start eval. Modernc apps use per-page bundles, so
// loading the bundle's render export warms the worker.
func (r *Renderer) Prime(bundlePaths []string) error {
	for _, path := range bundlePaths {
		source, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("prime SSR bundle %q: %w", path, err)
		}
		version := sha256.Sum256(source)
		for range len(r.workers) {
			w := <-r.workers
			render, err := w.evaluate(path, source, version)
			if err == nil {
				render.Free()
			}
			r.workers <- w
			if err != nil {
				return fmt.Errorf("prime SSR bundle %q: %w", path, err)
			}
		}
	}
	return nil
}

func (r *Renderer) Build(entrypoints []string, outdir string, entryNames []string) (map[string]core.ClientBuildResult, error) {
	if r.builder == nil {
		return nil, fmt.Errorf("modernc renderer has no build adapter")
	}
	return r.builder.Build(entrypoints, outdir, entryNames)
}

func (r *Renderer) BuildSSR(entrypoints []string, outdir string) error {
	if r.builder == nil {
		return fmt.Errorf("modernc renderer has no build adapter")
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
	w.undefined.Free()
	w.isPromiseFn.Free()
	_ = w.vm.Close()
}

func structuredRenderError(err error) error {
	if err == nil {
		return nil
	}

	message := err.Error()
	stack := ""
	var jsErr *quickjs.Error
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

const consoleShim = `
globalThis.console = {
	debug: (...args) => __bifrostLog(...args),
	info: (...args) => __bifrostLog(...args),
	log: (...args) => __bifrostLog(...args),
	warn: (...args) => __bifrostErr(...args),
	error: (...args) => __bifrostErr(...args),
};
`

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
