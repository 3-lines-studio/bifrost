package sobek

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
	js "github.com/3-lines-studio/sobek"
)

func BenchmarkSobekRealPageModulePreparation(b *testing.B) {
	source := optimizeReactStringAccumulator(realPageBenchmarkSource(b))
	const globalName = "BifrostPreparationBench"
	prebuilt := api.Transform(string(source), api.TransformOptions{
		Format:       api.FormatIIFE,
		GlobalName:   globalName,
		Target:       api.ES2015,
		MinifySyntax: true,
		Sourcefile:   "real-page-preparation-bench.js",
	})
	if len(prebuilt.Errors) > 0 {
		b.Fatalf("prebuild IIFE: %s", formatBuildMessages(prebuilt.Errors))
	}

	b.Run("RuntimeESMTransformAndCompile", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			transformed := api.Transform(string(source), api.TransformOptions{
				Format:       api.FormatIIFE,
				GlobalName:   globalName,
				Target:       api.ES2015,
				MinifySyntax: true,
				Sourcefile:   "real-page-preparation-bench.js",
			})
			if len(transformed.Errors) > 0 {
				b.Fatal(formatBuildMessages(transformed.Errors))
			}
			if _, err := js.Compile("real-page-preparation-bench.js", string(transformed.Code), true); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("PrebuiltIIFECompile", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if _, err := js.Compile("real-page-preparation-bench.js", string(prebuilt.Code), true); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkSobekRealPagePropsBoundary(b *testing.B) {
	source := optimizeReactStringAccumulator(realPageBenchmarkSource(b))
	w, render := benchmarkWorker(b, source, false)
	props := map[string]any{"name": "Benchmark"}
	propsJSON, err := json.Marshal(props)
	if err != nil {
		b.Fatal(err)
	}

	jsonPage := benchmarkRender(b, w, render, propsJSON)
	directPage := benchmarkRenderValue(b, w, render, w.vm.ToValue(props))
	if jsonPage != directPage {
		b.Fatalf("direct props changed render output\nJSON:   %+v\ndirect: %+v", jsonPage, directPage)
	}

	b.Run("JSONMarshalAndParse", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			encoded, err := json.Marshal(props)
			if err != nil {
				b.Fatal(err)
			}
			_ = benchmarkRender(b, w, render, encoded)
		}
	})
	b.Run("DirectGoValue", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_ = benchmarkRenderValue(b, w, render, w.vm.ToValue(props))
		}
	})
}

func BenchmarkSobekRealPageOutputStrategy(b *testing.B) {
	source := realPageBenchmarkSource(b)
	props, err := json.Marshal(map[string]any{"name": "Benchmark"})
	if err != nil {
		b.Fatal(err)
	}

	arrayWorker, arrayRender := benchmarkWorker(b, optimizeReactStringAccumulator(source), false)
	sinkSource, ok := optimizeReactGoOutputSink(source)
	if !ok {
		b.Fatal("real page bundle does not contain the expected React accumulator")
	}
	sinkWorker, sinkRender := benchmarkWorker(b, sinkSource, true)

	arrayPage := benchmarkRender(b, arrayWorker, arrayRender, props)
	sinkPage := benchmarkRender(b, sinkWorker, sinkRender, props)
	if arrayPage != sinkPage {
		b.Fatalf("output sink changed render output\narray: %+v\nsink:  %+v", arrayPage, sinkPage)
	}

	b.Run("ArrayJoin", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_ = benchmarkRender(b, arrayWorker, arrayRender, props)
		}
	})
	b.Run("GoStringsBuilder", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_ = benchmarkRender(b, sinkWorker, sinkRender, props)
		}
	})
}

type benchmarkPage struct {
	head string
	body string
}

func benchmarkWorker(tb testing.TB, source []byte, goSink bool) (*worker, js.Callable) {
	tb.Helper()
	globalName := "BifrostOptimizationBench"
	compiledSource := source
	if bytes.HasPrefix(bytes.TrimSpace(source), []byte(prebuiltIIFEMarker)) {
		globalName = prebuiltIIFEGlobal
	} else {
		transformed := api.Transform(string(source), api.TransformOptions{
			Format:       api.FormatIIFE,
			GlobalName:   globalName,
			Target:       api.ES2015,
			MinifySyntax: true,
			Sourcefile:   "real-page-optimization-bench.js",
		})
		if len(transformed.Errors) > 0 {
			tb.Fatalf("transform benchmark bundle: %s", formatBuildMessages(transformed.Errors))
		}
		compiledSource = transformed.Code
	}
	program, err := js.Compile("real-page-optimization-bench.js", string(compiledSource), true)
	if err != nil {
		tb.Fatal(err)
	}
	w, err := newWorker()
	if err != nil {
		tb.Fatal(err)
	}
	if goSink {
		installBenchmarkOutputSink(tb, w.vm)
	}
	if _, err := w.vm.RunProgram(program); err != nil {
		tb.Fatal(err)
	}
	exports := w.vm.Get(globalName).ToObject(w.vm)
	renderValue := exports.Get("render")
	if renderValue == nil || js.IsUndefined(renderValue) || js.IsNull(renderValue) {
		loaders := exports.Get("loaders")
		if loaders != nil && !js.IsUndefined(loaders) && !js.IsNull(loaders) {
			loader, ok := js.AssertFunction(loaders.ToObject(w.vm).Get("pages-home-entry-ssr"))
			if !ok {
				tb.Fatal("benchmark registry has no home loader")
			}
			renderValue, err = loader(js.Undefined())
			if err != nil {
				tb.Fatal(err)
			}
		} else {
			renders := exports.Get("renders")
			if renders != nil && !js.IsUndefined(renders) && !js.IsNull(renders) {
				renderValue = renders.ToObject(w.vm).Get("pages-home-entry-ssr")
			}
		}
	}
	render, ok := js.AssertFunction(renderValue)
	if !ok {
		tb.Fatal("benchmark bundle has no home render function")
	}
	return w, render
}

func installBenchmarkOutputSink(tb testing.TB, vm *js.Runtime) {
	tb.Helper()
	var output strings.Builder
	if err := vm.Set("__bifrostBeginOutput", func() {
		output.Reset()
	}); err != nil {
		tb.Fatal(err)
	}
	if err := vm.Set("__bifrostAppendOutput", func(chunk string) {
		output.WriteString(chunk)
	}); err != nil {
		tb.Fatal(err)
	}
	if err := vm.Set("__bifrostFinishOutput", func() string {
		return output.String()
	}); err != nil {
		tb.Fatal(err)
	}
}

func benchmarkRender(tb testing.TB, w *worker, render js.Callable, propsJSON []byte) benchmarkPage {
	tb.Helper()
	props, err := w.parse(js.Undefined(), w.vm.ToValue(string(propsJSON)))
	if err != nil {
		tb.Fatal(err)
	}
	return benchmarkRenderValue(tb, w, render, props)
}

func benchmarkRenderValue(tb testing.TB, w *worker, render js.Callable, props js.Value) benchmarkPage {
	tb.Helper()
	value, err := render(js.Undefined(), props)
	if err != nil {
		tb.Fatal(err)
	}
	value, err = settledValue(value)
	if err != nil {
		tb.Fatal(err)
	}
	result := value.ToObject(w.vm)
	return benchmarkPage{head: result.Get("head").String(), body: result.Get("html").String()}
}

var optimizedAccumulatorDeclaration = regexp.MustCompile(
	`var [A-Za-z_$][A-Za-z0-9_$]*=!1,[A-Za-z_$][A-Za-z0-9_$]*=null,([A-Za-z_$][A-Za-z0-9_$]*)=\[\],[A-Za-z_$][A-Za-z0-9_$]*=!1;`,
)

func optimizeReactGoOutputSink(source []byte) ([]byte, bool) {
	optimized := optimizeReactStringAccumulator(source)
	marker := bytes.Index(optimized, []byte(`The server used "renderToStaticMarkup"`))
	if marker < 0 {
		return nil, false
	}
	start := max(0, marker-2000)
	window := optimized[start:marker]
	matches := optimizedAccumulatorDeclaration.FindAllSubmatchIndex(window, -1)
	if len(matches) == 0 {
		return nil, false
	}
	match := matches[len(matches)-1]
	accumulator := string(window[match[2]:match[3]])
	push := regexp.MustCompile(
		`push:function\(([A-Za-z_$][A-Za-z0-9_$]*)\)\{return ([A-Za-z_$][A-Za-z0-9_$]*)!==null&&\(` +
			regexp.QuoteMeta(accumulator) + `\.push\(([A-Za-z_$][A-Za-z0-9_$]*)\)\),!0\}`,
	).FindSubmatch(window[match[1]:])
	if len(push) != 4 || !bytes.Equal(push[1], push[2]) || !bytes.Equal(push[1], push[3]) {
		return nil, false
	}
	chunk := string(push[1])
	initial := []byte(accumulator + `=[]`)
	finish := []byte(`return ` + accumulator + `.join("")}`)
	if bytes.Count(window, initial) != 1 || bytes.Count(window, push[0]) != 1 || bytes.Count(window, finish) != 1 {
		return nil, false
	}
	window = bytes.Replace(window, initial, []byte(accumulator+`=(__bifrostBeginOutput(),[])`), 1)
	window = bytes.Replace(
		window,
		push[0],
		[]byte(`push:function(`+chunk+`){return `+chunk+`!==null&&(__bifrostAppendOutput(`+chunk+`)),!0}`),
		1,
	)
	window = bytes.Replace(window, finish, []byte(`return __bifrostFinishOutput()}`), 1)
	result := make([]byte, 0, len(optimized))
	result = append(result, optimized[:start]...)
	result = append(result, window...)
	return append(result, optimized[marker:]...), true
}

func realPageBenchmarkSource(tb testing.TB) []byte {
	tb.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		tb.Fatal(err)
	}
	ssrDir := filepath.Join(repoRoot, "example", "cmd", "full", ".bifrost", "ssr")
	paths := []string{
		filepath.Join(ssrDir, "pages-home-entry-ssr.js"),
		filepath.Join(ssrDir, "bifrost-ssr-registry.js"),
	}
	for _, path := range paths {
		if source, err := os.ReadFile(path); err == nil {
			return source
		}
	}
	tb.Skip("real SSR benchmark bundle is unavailable; run 'make -C example build-sobek'")
	return nil
}
