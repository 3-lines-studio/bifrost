package sobek

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/evanw/esbuild/pkg/api"
	js "github.com/grafana/sobek"

	"github.com/3-lines-studio/bifrost/internal/core"
)

const renderTimeout = 30 * time.Second

type Builder interface {
	Build(entrypoints []string, outdir string, entryNames []string) (map[string]core.ClientBuildResult, error)
	BuildSSR(entrypoints []string, outdir string) error
}

type Renderer struct {
	mode    core.Mode
	builder Builder
	workers chan *worker
	modules moduleCache
	stopped atomic.Bool
}

type worker struct {
	vm      *js.Runtime
	parse   js.Callable
	modules map[string]loadedModule
}

type loadedModule struct {
	version [sha256.Size]byte
	render  js.Callable
}

type compiledModule struct {
	key        string
	version    [sha256.Size]byte
	program    *js.Program
	globalName string
}

type moduleCache struct {
	mu      sync.Mutex
	modules map[string]compiledModule
}

func NewRenderer(mode core.Mode, workers int, builder Builder) (*Renderer, error) {
	if workers <= 0 {
		workers = min(runtime.GOMAXPROCS(0), 4)
	}

	r := &Renderer{
		mode:    mode,
		builder: builder,
		workers: make(chan *worker, workers),
		modules: moduleCache{modules: make(map[string]compiledModule)},
	}
	for range workers {
		w, err := newWorker()
		if err != nil {
			return nil, fmt.Errorf("initialize Sobek worker: %w", err)
		}
		r.workers <- w
	}
	return r, nil
}

func newWorker() (*worker, error) {
	vm := js.New()
	if _, err := vm.RunString(`globalThis.queueMicrotask = (callback) => callback();`); err != nil {
		return nil, err
	}
	if err := installConsole(vm); err != nil {
		return nil, err
	}
	if err := installReactWebAPIs(vm); err != nil {
		return nil, err
	}
	jsonObject := vm.Get("JSON").ToObject(vm)
	parse, ok := js.AssertFunction(jsonObject.Get("parse"))
	if !ok {
		return nil, fmt.Errorf("JSON.parse is not callable")
	}
	return &worker{
		vm:      vm,
		parse:   parse,
		modules: make(map[string]loadedModule),
	}, nil
}

func installConsole(vm *js.Runtime) error {
	console := vm.NewObject()
	write := func(file *os.File) func(js.FunctionCall) js.Value {
		return func(call js.FunctionCall) js.Value {
			args := make([]any, len(call.Arguments))
			for i, argument := range call.Arguments {
				args[i] = argument.String()
			}
			_, _ = fmt.Fprintln(file, args...)
			return js.Undefined()
		}
	}
	for name, fn := range map[string]func(js.FunctionCall) js.Value{
		"debug": write(os.Stdout),
		"info":  write(os.Stdout),
		"log":   write(os.Stdout),
		"warn":  write(os.Stderr),
		"error": write(os.Stderr),
	} {
		if err := console.Set(name, fn); err != nil {
			return fmt.Errorf("install console.%s: %w", name, err)
		}
	}
	return vm.Set("console", console)
}

func installReactWebAPIs(vm *js.Runtime) error {
	if err := vm.Set("TextEncoder", func(call js.ConstructorCall) *js.Object {
		_ = call.This.Set("encode", func(encodeCall js.FunctionCall) js.Value {
			buffer := vm.NewArrayBuffer([]byte(encodeCall.Argument(0).String()))
			array, err := vm.New(vm.Get("Uint8Array"), vm.ToValue(buffer))
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return array
		})
		return nil
	}); err != nil {
		return fmt.Errorf("install TextEncoder: %w", err)
	}
	_, err := vm.RunString(`
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
	`)
	if err != nil {
		return fmt.Errorf("install MessageChannel: %w", err)
	}
	return nil
}

func (r *Renderer) Render(path string, props any) (core.RenderedPage, error) {
	return r.RenderContext(context.Background(), path, props)
}

func (r *Renderer) RenderContext(ctx context.Context, path string, props any) (core.RenderedPage, error) {
	if r == nil || r.stopped.Load() {
		return core.RenderedPage{}, fmt.Errorf("sobek renderer is stopped")
	}
	if path == "" {
		return core.RenderedPage{}, fmt.Errorf("missing SSR bundle path")
	}
	path = filePath(path)

	ctx, cancel := context.WithTimeout(ctx, renderTimeout)
	defer cancel()

	var w *worker
	select {
	case w = <-r.workers:
	case <-ctx.Done():
		return core.RenderedPage{}, ctx.Err()
	}
	defer func() { r.workers <- w }()

	module, err := r.modules.load(path, r.mode == core.ModeDev)
	if err != nil {
		return core.RenderedPage{}, err
	}
	render, err := w.load(module)
	if err != nil {
		return core.RenderedPage{}, structuredRenderError(err)
	}

	propsJSON, err := json.Marshal(props)
	if err != nil {
		return core.RenderedPage{}, err
	}
	propsValue, err := w.parse(js.Undefined(), w.vm.ToValue(string(propsJSON)))
	if err != nil {
		return core.RenderedPage{}, fmt.Errorf("parse render props: %w", err)
	}

	finished := make(chan struct{})
	interruptFinished := make(chan struct{})
	go func() {
		defer close(interruptFinished)
		select {
		case <-ctx.Done():
			w.vm.Interrupt(ctx.Err())
		case <-finished:
		}
	}()

	value, renderErr := render(js.Undefined(), propsValue)
	close(finished)
	<-interruptFinished
	w.vm.ClearInterrupt()
	if renderErr != nil {
		if ctx.Err() != nil {
			return core.RenderedPage{}, ctx.Err()
		}
		return core.RenderedPage{}, structuredRenderError(renderErr)
	}

	value, err = settledValue(value)
	if err != nil {
		return core.RenderedPage{}, structuredRenderError(err)
	}
	result := value.ToObject(w.vm)
	return core.RenderedPage{
		Head: result.Get("head").String(),
		Body: result.Get("html").String(),
	}, nil
}

func settledValue(value js.Value) (js.Value, error) {
	promise, ok := value.Export().(*js.Promise)
	if !ok {
		return value, nil
	}
	switch promise.State() {
	case js.PromiseStateFulfilled:
		return promise.Result(), nil
	case js.PromiseStateRejected:
		return nil, errorFromValue(promise.Result())
	default:
		return nil, fmt.Errorf("render returned a pending promise; asynchronous JavaScript SSR is unsupported")
	}
}

type javascriptError struct {
	message string
	stack   string
}

func (e *javascriptError) Error() string { return e.message }

func errorFromValue(value js.Value) error {
	result := &javascriptError{message: value.String()}
	if object, ok := value.(*js.Object); ok {
		if message := object.Get("message"); message != nil && !js.IsUndefined(message) {
			result.message = message.String()
		}
		if stack := object.Get("stack"); stack != nil && !js.IsUndefined(stack) {
			result.stack = stack.String()
		}
	}
	return result
}

func structuredRenderError(err error) error {
	if err == nil {
		return nil
	}

	message := err.Error()
	stack := ""
	var valueErr *javascriptError
	if errors.As(err, &valueErr) {
		message = valueErr.message
		stack = valueErr.stack
	}
	var exception *js.Exception
	if errors.As(err, &exception) {
		stack = exception.String()
		if object, ok := exception.Value().(*js.Object); ok {
			if value := object.Get("message"); value != nil && !js.IsUndefined(value) {
				message = value.String()
			}
		}
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

func filePath(value string) string {
	parsed, err := url.Parse(value)
	if err == nil && parsed.Scheme == "file" {
		return parsed.Path
	}
	return value
}

func (c *moduleCache) load(path string, reload bool) (compiledModule, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !reload {
		if module, ok := c.modules[path]; ok {
			return module, nil
		}
	}

	source, err := os.ReadFile(path)
	if err != nil {
		return compiledModule{}, fmt.Errorf("read SSR bundle %q: %w", path, err)
	}
	version := sha256.Sum256(source)
	source = optimizeReactStringAccumulator(source)
	if module, ok := c.modules[path]; ok && module.version == version {
		return module, nil
	}

	pathHash := sha256.Sum256([]byte(path))
	globalName := "BifrostSSR_" + hex.EncodeToString(pathHash[:8])
	transformed := api.Transform(string(source), api.TransformOptions{
		Format:       api.FormatIIFE,
		GlobalName:   globalName,
		Target:       api.ES2015,
		MinifySyntax: true,
		Sourcefile:   path,
		Sourcemap:    api.SourceMapInline,
	})
	if len(transformed.Errors) > 0 {
		return compiledModule{}, fmt.Errorf("transform SSR bundle %q: %s", path, formatBuildMessages(transformed.Errors))
	}
	program, err := js.Compile(path, string(transformed.Code), true)
	if err != nil {
		return compiledModule{}, fmt.Errorf("compile SSR bundle %q: %w", path, err)
	}
	module := compiledModule{key: path, version: version, program: program, globalName: globalName}
	c.modules[path] = module
	return module, nil
}

var reactAccumulatorDeclaration = regexp.MustCompile(
	`var [A-Za-z_$][A-Za-z0-9_$]*=!1,[A-Za-z_$][A-Za-z0-9_$]*=null,([A-Za-z_$][A-Za-z0-9_$]*)="",[A-Za-z_$][A-Za-z0-9_$]*=!1;`,
)

// optimizeReactStringAccumulator replaces React 19's repeated string concatenation
// with chunk collection. Sobek has flat strings, so the original loop is quadratic.
func optimizeReactStringAccumulator(source []byte) []byte {
	marker := bytes.Index(source, []byte(`The server used "renderToStaticMarkup"`))
	if marker < 0 {
		return source
	}
	start := max(0, marker-2000)
	window := source[start:marker]
	matches := reactAccumulatorDeclaration.FindAllSubmatchIndex(window, -1)
	if len(matches) == 0 {
		return source
	}
	match := matches[len(matches)-1]
	accumulator := string(window[match[2]:match[3]])

	appendExpression := regexp.MustCompile(
		`push:function\(([A-Za-z_$][A-Za-z0-9_$]*)\)\{return ([A-Za-z_$][A-Za-z0-9_$]*)!==null&&\(` +
			regexp.QuoteMeta(accumulator) + `\+=([A-Za-z_$][A-Za-z0-9_$]*)\),!0\}`,
	)
	appendMatch := appendExpression.FindSubmatch(window[match[1]:])
	if len(appendMatch) != 4 || !bytes.Equal(appendMatch[1], appendMatch[2]) || !bytes.Equal(appendMatch[1], appendMatch[3]) {
		return source
	}
	chunk := string(appendMatch[1])

	initial := []byte(accumulator + `=""`)
	appendChunk := appendMatch[0]
	returnResult := []byte(`return ` + accumulator + `}`)
	if bytes.Count(window, initial) != 1 || bytes.Count(window, appendChunk) != 1 || bytes.Count(window, returnResult) != 1 {
		return source
	}
	optimizedWindow := bytes.Replace(window, initial, []byte(accumulator+`=[]`), 1)
	optimizedWindow = bytes.Replace(
		optimizedWindow,
		appendChunk,
		[]byte(`push:function(`+chunk+`){return `+chunk+`!==null&&(`+accumulator+`.push(`+chunk+`)),!0}`),
		1,
	)
	optimizedWindow = bytes.Replace(
		optimizedWindow,
		returnResult,
		[]byte(`return `+accumulator+`.join("")}`),
		1,
	)
	optimized := make([]byte, 0, len(source))
	optimized = append(optimized, source[:start]...)
	optimized = append(optimized, optimizedWindow...)
	return append(optimized, source[marker:]...)
}

func formatBuildMessages(messages []api.Message) string {
	if len(messages) == 0 {
		return "unknown build error"
	}
	var out strings.Builder
	out.WriteString(messages[0].Text)
	for _, message := range messages[1:] {
		out.WriteString("; " + message.Text)
	}
	return out.String()
}

func (w *worker) load(module compiledModule) (js.Callable, error) {
	if loaded, ok := w.modules[module.key]; ok && loaded.version == module.version {
		return loaded.render, nil
	}
	if _, err := w.vm.RunProgram(module.program); err != nil {
		return nil, fmt.Errorf("evaluate SSR bundle: %w", err)
	}
	exports := w.vm.Get(module.globalName)
	if js.IsUndefined(exports) || js.IsNull(exports) {
		return nil, fmt.Errorf("SSR bundle did not define %s", module.globalName)
	}
	render, ok := js.AssertFunction(exports.ToObject(w.vm).Get("render"))
	if !ok {
		return nil, fmt.Errorf("SSR bundle did not export render")
	}
	w.modules[module.key] = loadedModule{version: module.version, render: render}
	return render, nil
}

func (r *Renderer) Build(entrypoints []string, outdir string, entryNames []string) (map[string]core.ClientBuildResult, error) {
	if r.builder == nil {
		return nil, fmt.Errorf("sobek renderer has no build adapter")
	}
	return r.builder.Build(entrypoints, outdir, entryNames)
}

func (r *Renderer) BuildSSR(entrypoints []string, outdir string) error {
	if r.builder == nil {
		return fmt.Errorf("sobek renderer has no build adapter")
	}
	return r.builder.BuildSSR(entrypoints, outdir)
}

func (r *Renderer) Stop() error {
	if r == nil || r.stopped.Swap(true) {
		return nil
	}
	if stopper, ok := r.builder.(interface{ Stop() error }); ok {
		return stopper.Stop()
	}
	return nil
}
