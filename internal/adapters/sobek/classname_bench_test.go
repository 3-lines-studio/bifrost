package sobek

import (
	"path/filepath"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
)

func BenchmarkSobekRealPageClassNameUtility(b *testing.B) {
	currentSource := optimizeReactStringAccumulator(realPageBenchmarkSource(b))
	simpleSource := optimizeReactStringAccumulator(buildSimpleClassNameRealPageBundle(b))
	currentWorker, currentRender := benchmarkWorker(b, currentSource, false)
	simpleWorker, simpleRender := benchmarkWorker(b, simpleSource, false)
	props := []byte(`{"name":"Benchmark"}`)

	currentPage := benchmarkRender(b, currentWorker, currentRender, props)
	simplePage := benchmarkRender(b, simpleWorker, simpleRender, props)
	parity := 0.0
	if currentPage == simplePage {
		parity = 1
	}

	b.Run("TailwindMerge", func(b *testing.B) {
		b.ReportAllocs()
		b.ReportMetric(1, "exact-parity")
		for range b.N {
			_ = benchmarkRender(b, currentWorker, currentRender, props)
		}
	})
	b.Run("SimpleStaticJoinUpperBound", func(b *testing.B) {
		b.ReportAllocs()
		b.ReportMetric(parity, "exact-parity")
		for range b.N {
			_ = benchmarkRender(b, simpleWorker, simpleRender, props)
		}
	})
}

func buildSimpleClassNameRealPageBundle(tb testing.TB) []byte {
	tb.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "example"))
	if err != nil {
		tb.Fatal(err)
	}
	legacyRenderer := filepath.Join(root, "node_modules", "react-dom", "cjs", "react-dom-server-legacy.browser.production.js")
	const source = `
import React from "react";
import {renderToString} from "react-dom/server";
import {Page, Head} from "./pages/home.tsx";
export function render(props) {
  return {
    head: Head ? renderToString(React.createElement(Head, props)) : "",
    html: renderToString(React.createElement(Page, props)),
  };
}`
	result := api.Build(api.BuildOptions{
		AbsWorkingDir:     root,
		Stdin:             &api.StdinOptions{Contents: source, ResolveDir: root, Sourcefile: "bifrost-classname-benchmark.tsx", Loader: api.LoaderTSX},
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
			Name: "classname-benchmark-resolver",
			Setup: func(build api.PluginBuild) {
				build.OnResolve(api.OnResolveOptions{Filter: `^react-dom/server$`}, func(api.OnResolveArgs) (api.OnResolveResult, error) {
					return api.OnResolveResult{Path: legacyRenderer}, nil
				})
				build.OnResolve(api.OnResolveOptions{Filter: `^@/lib/utils$`}, func(api.OnResolveArgs) (api.OnResolveResult, error) {
					return api.OnResolveResult{Path: "simple-cn", Namespace: "benchmark"}, nil
				})
				build.OnLoad(api.OnLoadOptions{Filter: `^simple-cn$`, Namespace: "benchmark"}, func(api.OnLoadArgs) (api.OnLoadResult, error) {
					code := `export function cn(...inputs) { return inputs.filter(Boolean).join(" "); }`
					return api.OnLoadResult{Contents: &code, Loader: api.LoaderJS}, nil
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
		tb.Fatalf("build classname benchmark: %s", formatBuildMessages(result.Errors))
	}
	if len(result.OutputFiles) != 1 {
		tb.Fatalf("classname outputs = %d, want 1", len(result.OutputFiles))
	}
	return result.OutputFiles[0].Contents
}
