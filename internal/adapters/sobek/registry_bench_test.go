package sobek

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	js "github.com/3-lines-studio/sobek"
	"github.com/evanw/esbuild/pkg/api"
)

type registryBenchmarkPage struct {
	id        string
	component string
	bundle    string
	propsJSON string
}

type registryBenchmarkModule struct {
	program    *js.Program
	globalName string
	bytes      int
}

func BenchmarkSobekProductionRegistry(b *testing.B) {
	pages, root := registryBenchmarkPages(b)
	separate := make([]registryBenchmarkModule, len(pages))
	separateBytes := 0
	for i, page := range pages {
		source, err := os.ReadFile(page.bundle)
		if err != nil {
			source = buildRegistryIndividualBundle(b, root, page.component)
		}
		separate[i] = compileRegistryBenchmarkModule(b, source, fmt.Sprintf("Separate%d", i))
		separateBytes += len(source)
	}
	registrySource := buildRegistryBenchmarkBundle(b, root, pages)
	registry := compileRegistryBenchmarkModule(b, registrySource, "Registry")

	assertRegistryBenchmarkParity(b, pages, separate, registry)

	b.Run("WorkerWarmupAllPages/SeparateBundles", func(b *testing.B) {
		b.ReportAllocs()
		b.ReportMetric(float64(separateBytes), "bundle-B")
		for range b.N {
			w, err := newWorker()
			if err != nil {
				b.Fatal(err)
			}
			for i, module := range separate {
				render := runRegistryBenchmarkModule(b, w, module)
				props := parseRegistryProps(b, w, pages[i].propsJSON)
				if _, err := render(js.Undefined(), props); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("WorkerWarmupAllPages/CombinedRegistry", func(b *testing.B) {
		b.ReportAllocs()
		b.ReportMetric(float64(registry.bytes), "bundle-B")
		for range b.N {
			w, err := newWorker()
			if err != nil {
				b.Fatal(err)
			}
			runRegistryBenchmarkProgram(b, w, registry)
			for _, page := range pages {
				render := registryBenchmarkPageRender(b, w, registry, page.id)
				props := parseRegistryProps(b, w, page.propsJSON)
				if _, err := render(js.Undefined(), props); err != nil {
					b.Fatal(err)
				}
			}
		}
	})

	b.Run("WarmHome/SeparateBundle", func(b *testing.B) {
		w, err := newWorker()
		if err != nil {
			b.Fatal(err)
		}
		render := runRegistryBenchmarkModule(b, w, separate[0])
		props := parseRegistryProps(b, w, pages[0].propsJSON)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if _, err := render(js.Undefined(), props); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("WarmHome/CombinedRegistry", func(b *testing.B) {
		w, err := newWorker()
		if err != nil {
			b.Fatal(err)
		}
		runRegistryBenchmarkProgram(b, w, registry)
		render := registryBenchmarkPageRender(b, w, registry, pages[0].id)
		props := parseRegistryProps(b, w, pages[0].propsJSON)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if _, err := render(js.Undefined(), props); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func registryBenchmarkPages(tb testing.TB) ([]registryBenchmarkPage, string) {
	tb.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "example"))
	if err != nil {
		tb.Fatal(err)
	}
	ssr := filepath.Join(root, "cmd", "full", ".bifrost", "ssr")
	pages := []registryBenchmarkPage{
		{id: "home", component: "./pages/home.tsx", bundle: filepath.Join(ssr, "pages-home-entry-ssr.js"), propsJSON: `{"name":"Benchmark"}`},
		{id: "nested", component: "./pages/nested/page.tsx", bundle: filepath.Join(ssr, "pages-nested-page-entry-ssr.js"), propsJSON: `{"name":"Nested"}`},
		{id: "api", component: "./pages/api-demo.tsx", bundle: filepath.Join(ssr, "pages-api-demo-entry-ssr.js"), propsJSON: `{"users":[],"loadTime":"now"}`},
		{id: "dashboard", component: "./pages/dashboard.tsx", bundle: filepath.Join(ssr, "pages-dashboard-entry-ssr.js"), propsJSON: `{"user":{"name":"Benchmark","role":"admin"}}`},
	}
	return pages, root
}

func buildRegistryIndividualBundle(tb testing.TB, root, component string) []byte {
	tb.Helper()
	legacyRenderer := filepath.Join(root, "node_modules", "react-dom", "cjs", "react-dom-server-legacy.browser.production.js")
	source := fmt.Sprintf(`
import React from "react";
import {renderToString} from "react-dom/server";
import {Page, Head} from %q;
export function render(props) {
  return {
    head: Head ? renderToString(React.createElement(Head, props)) : "",
    html: renderToString(React.createElement(Page, props)),
  };
}`, component)
	result := api.Build(api.BuildOptions{
		AbsWorkingDir:     root,
		Stdin:             &api.StdinOptions{Contents: source, ResolveDir: root, Sourcefile: "registry-individual-benchmark.tsx", Loader: api.LoaderTSX},
		Bundle:            true,
		Write:             false,
		Platform:          api.PlatformBrowser,
		Format:            api.FormatESModule,
		Target:            api.ES2015,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
		Define:            map[string]string{"process.env.NODE_ENV": `"production"`},
		Plugins: []api.Plugin{{
			Name: "registry-individual-benchmark-resolver",
			Setup: func(build api.PluginBuild) {
				build.OnResolve(api.OnResolveOptions{Filter: `^react-dom/server$`}, func(api.OnResolveArgs) (api.OnResolveResult, error) {
					return api.OnResolveResult{Path: legacyRenderer}, nil
				})
				build.OnLoad(api.OnLoadOptions{Filter: `\.css$`}, func(api.OnLoadArgs) (api.OnLoadResult, error) {
					empty := ""
					return api.OnLoadResult{Contents: &empty, Loader: api.LoaderEmpty}, nil
				})
			},
		}},
		LogLevel: api.LogLevelSilent,
	})
	if len(result.Errors) > 0 {
		tb.Fatalf("build individual registry benchmark: %s", formatBuildMessages(result.Errors))
	}
	if len(result.OutputFiles) != 1 {
		tb.Fatalf("individual registry outputs = %d, want 1", len(result.OutputFiles))
	}
	return result.OutputFiles[0].Contents
}

func buildRegistryBenchmarkBundle(tb testing.TB, root string, pages []registryBenchmarkPage) []byte {
	tb.Helper()
	var source strings.Builder
	source.WriteString(`import React from "react"; import {renderToString} from "react-dom/server";`)
	for i, page := range pages {
		fmt.Fprintf(&source, `import {Page as Page%d, Head as Head%d} from %q;`, i, i, page.component)
	}
	source.WriteString(`const pages={`)
	for i, page := range pages {
		fmt.Fprintf(&source, `%q:{Page:Page%d,Head:Head%d},`, page.id, i, i)
	}
	source.WriteString(`}; function renderEntry(entry,props){let head="";if(entry.Head)head=renderToString(React.createElement(entry.Head,props));return {head,html:renderToString(React.createElement(entry.Page,props))};} export function render(id,props){return renderEntry(pages[id],props)} export const renders={`)
	for _, page := range pages {
		fmt.Fprintf(&source, `%q:(props)=>renderEntry(pages[%q],props),`, page.id, page.id)
	}
	source.WriteString(`};`)

	legacyRenderer := filepath.Join(root, "node_modules", "react-dom", "cjs", "react-dom-server-legacy.browser.production.js")
	result := api.Build(api.BuildOptions{
		AbsWorkingDir: root,
		Stdin: &api.StdinOptions{
			Contents:   source.String(),
			ResolveDir: root,
			Sourcefile: "bifrost-registry-benchmark.tsx",
			Loader:     api.LoaderTSX,
		},
		Bundle:            true,
		Write:             false,
		Platform:          api.PlatformBrowser,
		Format:            api.FormatESModule,
		Target:            api.ES2015,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
		Define: map[string]string{
			"process.env.NODE_ENV": `"production"`,
		},
		Plugins: []api.Plugin{{
			Name: "registry-benchmark-resolver",
			Setup: func(build api.PluginBuild) {
				build.OnResolve(api.OnResolveOptions{Filter: `^react-dom/server$`}, func(api.OnResolveArgs) (api.OnResolveResult, error) {
					return api.OnResolveResult{Path: legacyRenderer}, nil
				})
				build.OnLoad(api.OnLoadOptions{Filter: `\.css$`}, func(api.OnLoadArgs) (api.OnLoadResult, error) {
					empty := ""
					return api.OnLoadResult{Contents: &empty, Loader: api.LoaderEmpty}, nil
				})
			},
		}},
		LogLevel: api.LogLevelSilent,
	})
	if len(result.Errors) > 0 {
		tb.Fatalf("build registry benchmark: %s", formatBuildMessages(result.Errors))
	}
	if len(result.OutputFiles) != 1 {
		tb.Fatalf("registry outputs = %d, want 1", len(result.OutputFiles))
	}
	return result.OutputFiles[0].Contents
}

func compileRegistryBenchmarkModule(tb testing.TB, source []byte, globalName string) registryBenchmarkModule {
	tb.Helper()
	source = optimizeReactStringAccumulator(source)
	compiledSource := source
	if bytes.HasPrefix(bytes.TrimSpace(source), []byte(prebuiltIIFEMarker)) {
		globalName = prebuiltIIFEGlobal
	} else {
		transformed := api.Transform(string(source), api.TransformOptions{
			Format:       api.FormatIIFE,
			GlobalName:   globalName,
			Target:       api.ES2015,
			MinifySyntax: true,
			Sourcefile:   globalName + ".js",
		})
		if len(transformed.Errors) > 0 {
			tb.Fatalf("transform %s: %s", globalName, formatBuildMessages(transformed.Errors))
		}
		compiledSource = transformed.Code
	}
	program, err := js.Compile(globalName+".js", string(compiledSource), true)
	if err != nil {
		tb.Fatal(err)
	}
	return registryBenchmarkModule{program: program, globalName: globalName, bytes: len(source)}
}

func runRegistryBenchmarkProgram(tb testing.TB, w *worker, module registryBenchmarkModule) {
	tb.Helper()
	if _, err := w.vm.RunProgram(module.program); err != nil {
		tb.Fatal(err)
	}
}

func runRegistryBenchmarkModule(tb testing.TB, w *worker, module registryBenchmarkModule) js.Callable {
	tb.Helper()
	runRegistryBenchmarkProgram(tb, w, module)
	render, ok := js.AssertFunction(w.vm.Get(module.globalName).ToObject(w.vm).Get("render"))
	if !ok {
		tb.Fatalf("%s has no render function", module.globalName)
	}
	return render
}

func registryBenchmarkPageRender(tb testing.TB, w *worker, module registryBenchmarkModule, id string) js.Callable {
	tb.Helper()
	exports := w.vm.Get(module.globalName).ToObject(w.vm)
	renders := exports.Get("renders").ToObject(w.vm)
	render, ok := js.AssertFunction(renders.Get(id))
	if !ok {
		tb.Fatalf("%s has no registry render function for %s", module.globalName, id)
	}
	return render
}

func parseRegistryProps(tb testing.TB, w *worker, propsJSON string) js.Value {
	tb.Helper()
	props, err := w.parse(js.Undefined(), w.vm.ToValue(propsJSON))
	if err != nil {
		tb.Fatal(err)
	}
	return props
}

func assertRegistryBenchmarkParity(tb testing.TB, pages []registryBenchmarkPage, separate []registryBenchmarkModule, registry registryBenchmarkModule) {
	tb.Helper()
	separateWorker, err := newWorker()
	if err != nil {
		tb.Fatal(err)
	}
	registryWorker, err := newWorker()
	if err != nil {
		tb.Fatal(err)
	}
	runRegistryBenchmarkProgram(tb, registryWorker, registry)
	for i, page := range pages {
		separateRender := runRegistryBenchmarkModule(tb, separateWorker, separate[i])
		separateProps := parseRegistryProps(tb, separateWorker, page.propsJSON)
		registryProps := parseRegistryProps(tb, registryWorker, page.propsJSON)
		separateValue, err := separateRender(js.Undefined(), separateProps)
		if err != nil {
			tb.Fatal(err)
		}
		registryRender := registryBenchmarkPageRender(tb, registryWorker, registry, page.id)
		registryValue, err := registryRender(js.Undefined(), registryProps)
		if err != nil {
			tb.Fatal(err)
		}
		separateResult := separateValue.ToObject(separateWorker.vm)
		registryResult := registryValue.ToObject(registryWorker.vm)
		if separateResult.Get("head").String() != registryResult.Get("head").String() || separateResult.Get("html").String() != registryResult.Get("html").String() {
			tb.Fatalf("registry output differs for %s", page.id)
		}
	}
}
