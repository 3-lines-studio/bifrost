package process

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/grafana/sobek"

	"github.com/3-lines-studio/bifrost/internal/core"
)

const reactEngineFixtureSource = `
import React, { createContext, useContext, useId } from "react";
import { renderToString } from "bifrost-react-dom-server";

const Theme = createContext("missing");

function Child({ label }) {
  const id = useId();
  const theme = useContext(Theme);
  return (
    <section id={id} data-theme={theme}>
      <span>{label}</span>
      <p>{"<escaped>&"}</p>
    </section>
  );
}

function Page(props) {
  return (
    <Theme.Provider value={props.theme}>
      <main className="page">
        <h1>Hello {props.name}</h1>
        <Child label={props.label} />
      </main>
    </Theme.Provider>
  );
}

function Head(props) {
  return (
    <>
      <title>{props.title}</title>
      <meta name="description" content={props.description} />
    </>
  );
}

export function render(propsInput) {
  const wantsJSON = typeof propsInput === "string";
  const props = wantsJSON ? JSON.parse(propsInput) : propsInput;
  const result = {
    head: renderToString(<Head {...props} />),
    html: renderToString(<Page {...props} />),
  };
  return wantsJSON ? JSON.stringify(result) : result;
}
`

var reactEngineFixtureProps = map[string]string{
	"name":        "世界",
	"label":       "Child",
	"theme":       "dark",
	"title":       "Bench & Test",
	"description": "A < B",
}

type engineRenderedPage struct {
	Head string `json:"head"`
	HTML string `json:"html"`
}

func TestEsbuildReactBundleMatchesBunAndSobek(t *testing.T) {
	bundle := buildReactEngineFixture(t, api.FormatIIFE)
	propsJSON := marshalEngineFixtureProps(t)

	sobekPage := renderFixtureWithSobek(t, bundle, propsJSON)
	bunPage := renderFixtureWithBun(t, bundle, propsJSON)

	if sobekPage != bunPage {
		t.Fatalf("Sobek output differs from Bun\nSobek: %+v\nBun:   %+v", sobekPage, bunPage)
	}

	want := engineRenderedPage{
		Head: `<title>Bench &amp; Test</title><meta name="description" content="A &lt; B"/>`,
		HTML: `<main class="page"><h1>Hello <!-- -->世界</h1><section id="_R_2_" data-theme="dark"><span>Child</span><p>&lt;escaped&gt;&amp;</p></section></main>`,
	}
	if sobekPage != want {
		t.Fatalf("render output changed\ngot:  %+v\nwant: %+v", sobekPage, want)
	}
}

func BenchmarkReactEngineRender(b *testing.B) {
	bundle := buildReactEngineFixture(b, api.FormatIIFE)
	propsJSON := marshalEngineFixtureProps(b)

	b.Run("Sobek", func(b *testing.B) {
		vm, render := newSobekFixtureRenderer(b, bundle)
		props := vm.ToValue(propsJSON)

		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if _, err := render(sobek.Undefined(), props); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("BunSidecar", func(b *testing.B) {
		if _, err := exec.LookPath("bun"); err != nil {
			b.Skip("bun is not available")
		}

		module := buildReactEngineFixture(b, api.FormatESModule)
		componentPath := filepath.Join(b.TempDir(), "react-engine-fixture.mjs")
		if err := os.WriteFile(componentPath, module, 0o644); err != nil {
			b.Fatal(err)
		}

		renderer, err := NewRenderer(core.ModeProd, RuntimeSource(core.ModeProd), "BIFROST_DEV=0")
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { _ = renderer.Stop() })

		if _, err := renderer.Render(componentPath, reactEngineFixtureProps); err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if _, err := renderer.Render(componentPath, reactEngineFixtureProps); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkReactEngineStartupAndFirstRender(b *testing.B) {
	bundle := buildReactEngineFixture(b, api.FormatIIFE)
	propsJSON := marshalEngineFixtureProps(b)
	program, err := sobek.Compile("bifrost-react-ssr.js", string(bundle), true)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("Sobek", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			vm := sobek.New()
			if _, err := vm.RunProgram(program); err != nil {
				b.Fatal(err)
			}
			render := sobekRenderFunction(b, vm)
			if _, err := render(sobek.Undefined(), vm.ToValue(propsJSON)); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("BunSidecar", func(b *testing.B) {
		if _, err := exec.LookPath("bun"); err != nil {
			b.Skip("bun is not available")
		}

		module := buildReactEngineFixture(b, api.FormatESModule)
		componentPath := filepath.Join(b.TempDir(), "react-engine-fixture.mjs")
		if err := os.WriteFile(componentPath, module, 0o644); err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		for range b.N {
			renderer, err := NewRenderer(core.ModeProd, RuntimeSource(core.ModeProd), "BIFROST_DEV=0")
			if err != nil {
				b.Fatal(err)
			}
			if _, err := renderer.Render(componentPath, reactEngineFixtureProps); err != nil {
				_ = renderer.Stop()
				b.Fatal(err)
			}
			if err := renderer.Stop(); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func buildReactEngineFixture(tb testing.TB, format api.Format) []byte {
	tb.Helper()

	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		tb.Fatal(err)
	}
	fixtureRoot := filepath.Join(repoRoot, "example")
	legacyRenderer := filepath.Join(
		fixtureRoot,
		"node_modules",
		"react-dom",
		"cjs",
		"react-dom-server-legacy.browser.production.js",
	)
	if _, err := os.Stat(legacyRenderer); err != nil {
		tb.Skipf("React fixture dependencies are unavailable; run 'cd example && bun install': %v", err)
	}

	result := api.Build(api.BuildOptions{
		AbsWorkingDir: fixtureRoot,
		Stdin: &api.StdinOptions{
			Contents:   reactEngineFixtureSource,
			ResolveDir: fixtureRoot,
			Sourcefile: "react-engine-fixture.tsx",
			Loader:     api.LoaderTSX,
		},
		Bundle:            true,
		Write:             false,
		Format:            format,
		GlobalName:        "BifrostSSR",
		Platform:          api.PlatformBrowser,
		Target:            api.ES2015,
		MinifySyntax:      true,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		Define: map[string]string{
			"process.env.NODE_ENV": `"production"`,
		},
		Plugins: []api.Plugin{{
			Name: "bifrost-react-dom-server",
			Setup: func(build api.PluginBuild) {
				build.OnResolve(api.OnResolveOptions{Filter: `^bifrost-react-dom-server$`}, func(api.OnResolveArgs) (api.OnResolveResult, error) {
					return api.OnResolveResult{Path: legacyRenderer}, nil
				})
			},
		}},
	})
	if len(result.Errors) > 0 {
		tb.Fatalf("esbuild failed: %s", formatEsbuildMessages(result.Errors))
	}
	if len(result.OutputFiles) != 1 {
		tb.Fatalf("esbuild outputs = %d, want 1", len(result.OutputFiles))
	}
	return result.OutputFiles[0].Contents
}

func formatEsbuildMessages(messages []api.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		parts = append(parts, message.Text)
	}
	return strings.Join(parts, "; ")
}

func marshalEngineFixtureProps(tb testing.TB) string {
	tb.Helper()
	data, err := json.Marshal(reactEngineFixtureProps)
	if err != nil {
		tb.Fatal(err)
	}
	return string(data)
}

func newSobekFixtureRenderer(tb testing.TB, bundle []byte) (*sobek.Runtime, sobek.Callable) {
	tb.Helper()
	program, err := sobek.Compile("bifrost-react-ssr.js", string(bundle), true)
	if err != nil {
		tb.Fatal(err)
	}
	vm := sobek.New()
	if _, err := vm.RunProgram(program); err != nil {
		tb.Fatal(err)
	}
	return vm, sobekRenderFunction(tb, vm)
}

func sobekRenderFunction(tb testing.TB, vm *sobek.Runtime) sobek.Callable {
	tb.Helper()
	ssr := vm.Get("BifrostSSR")
	if sobek.IsUndefined(ssr) || sobek.IsNull(ssr) {
		tb.Fatal("SSR bundle did not define BifrostSSR")
	}
	render, ok := sobek.AssertFunction(ssr.ToObject(vm).Get("render"))
	if !ok {
		tb.Fatal("SSR bundle did not export render")
	}
	return render
}

func renderFixtureWithSobek(tb testing.TB, bundle []byte, propsJSON string) engineRenderedPage {
	tb.Helper()
	vm, render := newSobekFixtureRenderer(tb, bundle)
	value, err := render(sobek.Undefined(), vm.ToValue(propsJSON))
	if err != nil {
		tb.Fatal(err)
	}
	return decodeEngineRenderedPage(tb, value.String())
}

func renderFixtureWithBun(tb testing.TB, bundle []byte, propsJSON string) engineRenderedPage {
	tb.Helper()
	if _, err := exec.LookPath("bun"); err != nil {
		tb.Skip("bun is not available")
	}

	scriptPath := filepath.Join(tb.TempDir(), "render-fixture.js")
	script := string(bundle) + "\nconsole.log(BifrostSSR.render(" + fmt.Sprintf("%q", propsJSON) + "));\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		tb.Fatal(err)
	}
	cmd := exec.Command("bun", scriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		tb.Fatalf("Bun render failed: %v\n%s", err, output)
	}
	return decodeEngineRenderedPage(tb, strings.TrimSpace(string(output)))
}

func decodeEngineRenderedPage(tb testing.TB, value string) engineRenderedPage {
	tb.Helper()
	var page engineRenderedPage
	if err := json.Unmarshal([]byte(value), &page); err != nil {
		tb.Fatalf("decode rendered page: %v\n%s", err, value)
	}
	return page
}
