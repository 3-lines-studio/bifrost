package sobek

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestRendererRendersESMBundle(t *testing.T) {
	bundle := writeBundle(t, `
export async function render(props) {
  return {
    head: "<title>" + props.title + "</title>",
    html: "<main>Hello " + props.name + "</main>",
  };
}
`)
	renderer, err := NewRenderer(core.ModeProd, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	page, err := renderer.Render(bundle, map[string]any{"title": "Test", "name": "World"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Head != "<title>Test</title>" {
		t.Fatalf("head = %q", page.Head)
	}
	if page.Body != "<main>Hello World</main>" {
		t.Fatalf("body = %q", page.Body)
	}
}

func TestRendererRendersPrebuiltIIFEBundle(t *testing.T) {
	bundle := writeBundle(t, prebuiltIIFEMarker+`
var __BIFROST_SSR__ = (() => ({
  render(props) {
    return { head: "<title>" + props.title + "</title>", html: "<main>" + props.name + "</main>" };
  }
}))();
`)
	renderer, err := NewRenderer(core.ModeProd, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	page, err := renderer.Render(bundle, map[string]any{"title": "IIFE", "name": "Ready"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Head != "<title>IIFE</title>" || page.Body != "<main>Ready</main>" {
		t.Fatalf("unexpected page: %+v", page)
	}
}

func TestRendererSelectsRenderFromPrebuiltRegistry(t *testing.T) {
	bundle := writeBundle(t, prebuiltIIFEMarker+`
var __BIFROST_SSR__ = (() => ({
  renders: {
    home(props) { return { head: "home", html: props.value }; },
    about(props) { return { head: "about", html: props.value }; },
  }
}))();
`)
	renderer, err := NewRenderer(core.ModeProd, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	home, err := renderer.Render(bundle+"#home", map[string]any{"value": "first"})
	if err != nil {
		t.Fatal(err)
	}
	about, err := renderer.Render(bundle+"#about", map[string]any{"value": "second"})
	if err != nil {
		t.Fatal(err)
	}
	if home.Head != "home" || home.Body != "first" || about.Head != "about" || about.Body != "second" {
		t.Fatalf("unexpected registry pages: home=%+v about=%+v", home, about)
	}
}

func TestRendererReloadsChangedBundleInDev(t *testing.T) {
	bundle := writeBundle(t, `export function render() { return { head: "one", html: "one" }; }`)
	renderer, err := NewRenderer(core.ModeDev, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	first, err := renderer.Render(bundle, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Body != "one" {
		t.Fatalf("first body = %q", first.Body)
	}

	if err := os.WriteFile(bundle, []byte(`export function render() { return { head: "two", html: "two" }; }`), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := renderer.Render(bundle, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.Body != "two" {
		t.Fatalf("second body = %q", second.Body)
	}
}

func TestRendererInterruptsCanceledRenderAndRecovers(t *testing.T) {
	loopBundle := writeBundle(t, `export function render() { while (true) {} }`)
	goodBundle := writeBundle(t, `export function render() { return { head: "", html: "recovered" }; }`)
	renderer, err := NewRenderer(core.ModeProd, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = renderer.RenderContext(ctx, loopBundle, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}

	page, err := renderer.Render(goodBundle, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.Body != "recovered" {
		t.Fatalf("body = %q", page.Body)
	}
}

func TestRendererRejectsPendingPromise(t *testing.T) {
	bundle := writeBundle(t, `export function render() { return new Promise(() => {}); }`)
	renderer, err := NewRenderer(core.ModeProd, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = renderer.Render(bundle, nil)
	if err == nil || !strings.Contains(err.Error(), "pending promise") {
		t.Fatalf("error = %v", err)
	}
}

func TestOptimizeReactStringAccumulator(t *testing.T) {
	source := []byte(`before;var n=!1,o=null,u="",c=!1;push:function(y){return y!==null&&(u+=y),!0};return u}var i1=The server used "renderToStaticMarkup";after`)
	got := string(optimizeReactStringAccumulator(source))
	for _, want := range []string{`u=[]`, `u.push(y)`, `return u.join("")`} {
		if !strings.Contains(got, want) {
			t.Fatalf("optimized source does not contain %q: %s", want, got)
		}
	}
}

func TestRendererDelegatesBuilds(t *testing.T) {
	builder := &fakeBuilder{}
	renderer, err := NewRenderer(core.ModeProd, 1, builder)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := renderer.Build([]string{"client.tsx"}, "dist", []string{"client"}); err != nil {
		t.Fatal(err)
	}
	if err := renderer.BuildSSR([]string{"server.tsx"}, "ssr"); err != nil {
		t.Fatal(err)
	}
	if builder.clientCalls != 1 || builder.ssrCalls != 1 {
		t.Fatalf("build calls = client %d, SSR %d", builder.clientCalls, builder.ssrCalls)
	}
}

type fakeBuilder struct {
	clientCalls int
	ssrCalls    int
}

func (b *fakeBuilder) Build(_ []string, _ string, _ []string) (map[string]core.ClientBuildResult, error) {
	b.clientCalls++
	return map[string]core.ClientBuildResult{}, nil
}

func (b *fakeBuilder) BuildSSR(_ []string, _ string) error {
	b.ssrCalls++
	return nil
}

func writeBundle(t *testing.T, source string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "page.js")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
