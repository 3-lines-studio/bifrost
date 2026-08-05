package sobek

import (
	"path/filepath"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
)

func BenchmarkSobekReactVsPreactRealPage(b *testing.B) {
	reactSource := optimizeReactStringAccumulator(realPageBenchmarkSource(b))
	preactSource := buildPreactRealPageBundle(b)
	reactWorker, reactRender := benchmarkWorker(b, reactSource, false)
	preactWorker, preactRender := benchmarkWorker(b, preactSource, false)
	props := []byte(`{"name":"Benchmark"}`)

	reactPage := benchmarkRender(b, reactWorker, reactRender, props)
	preactPage := benchmarkRender(b, preactWorker, preactRender, props)
	parity := 0.0
	if reactPage == preactPage {
		parity = 1
	}

	b.Run("React19", func(b *testing.B) {
		b.ReportAllocs()
		b.ReportMetric(1, "exact-parity")
		for range b.N {
			_ = benchmarkRender(b, reactWorker, reactRender, props)
		}
	})
	b.Run("PreactCompat", func(b *testing.B) {
		b.ReportAllocs()
		b.ReportMetric(parity, "exact-parity")
		for range b.N {
			_ = benchmarkRender(b, preactWorker, preactRender, props)
		}
	})
}

func buildPreactRealPageBundle(tb testing.TB) []byte {
	tb.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "example"))
	if err != nil {
		tb.Fatal(err)
	}
	const source = `
import React from "preact/compat";
import renderToString from "preact-render-to-string";
import {Page, Head} from "./pages/home.tsx";
export function render(props) {
  return {
    head: Head ? renderToString(React.createElement(Head, props)) : "",
    html: renderToString(React.createElement(Page, props)),
  };
}`
	result := api.Build(api.BuildOptions{
		AbsWorkingDir: root,
		Stdin: &api.StdinOptions{
			Contents:   source,
			ResolveDir: root,
			Sourcefile: "bifrost-preact-benchmark.tsx",
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
		Alias: map[string]string{
			"react":                 "preact/compat",
			"react-dom":             "preact/compat",
			"react/jsx-runtime":     "preact/jsx-runtime",
			"react/jsx-dev-runtime": "preact/jsx-dev-runtime",
		},
		Plugins: []api.Plugin{{
			Name: "preact-benchmark-ignore-css",
			Setup: func(build api.PluginBuild) {
				build.OnLoad(api.OnLoadOptions{Filter: `\.css$`}, func(api.OnLoadArgs) (api.OnLoadResult, error) {
					empty := ""
					return api.OnLoadResult{Contents: &empty, Loader: api.LoaderEmpty}, nil
				})
			},
		}},
		Define:   map[string]string{"process.env.NODE_ENV": `"production"`},
		LogLevel: api.LogLevelSilent,
	})
	if len(result.Errors) > 0 {
		tb.Fatalf("build Preact benchmark: %s", formatBuildMessages(result.Errors))
	}
	if len(result.OutputFiles) != 1 {
		tb.Fatalf("Preact outputs = %d, want 1", len(result.OutputFiles))
	}
	return result.OutputFiles[0].Contents
}
