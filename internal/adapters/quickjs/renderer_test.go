package quickjs

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
	defer func() { _ = renderer.Stop() }()

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
	defer func() { _ = renderer.Stop() }()

	page, err := renderer.Render(bundle, map[string]any{"title": "IIFE", "name": "Ready"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Head != "<title>IIFE</title>" || page.Body != "<main>Ready</main>" {
		t.Fatalf("unexpected page: %+v", page)
	}
}

func TestRendererReloadsChangedBundleInDev(t *testing.T) {
	bundle := writeBundle(t, `export function render() { return { head: "one", html: "one" }; }`)
	renderer, err := NewRenderer(core.ModeDev, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = renderer.Stop() }()

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
	defer func() { _ = renderer.Stop() }()

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

func TestRendererRejectsRegistryPath(t *testing.T) {
	bundle := writeBundle(t, `export function render() { return { head: "", html: "" }; }`)
	renderer, err := NewRenderer(core.ModeProd, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = renderer.Stop() }()

	_, err = renderer.Render(bundle+"#pages-home-entry-ssr", nil)
	if err == nil || !strings.Contains(err.Error(), "registry") {
		t.Fatalf("error = %v, want registry unsupported", err)
	}
}

func TestRendererRejectsPendingPromise(t *testing.T) {
	bundle := writeBundle(t, `export function render() { return new Promise(() => {}); }`)
	renderer, err := NewRenderer(core.ModeProd, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = renderer.Stop() }()

	done := make(chan error, 1)
	go func() {
		_, err := renderer.Render(bundle, nil)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "pending promise") {
			t.Fatalf("error = %v, want pending promise rejection", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("render hung on a pending promise")
	}
}

func TestRendererFailsWhenReloadedBundleDefinesNoGlobal(t *testing.T) {
	bundle := writeBundle(t, `export function render() { return { head: "", html: "one" }; }`)
	renderer, err := NewRenderer(core.ModeDev, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = renderer.Stop() }()

	if _, err := renderer.Render(bundle, nil); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(bundle, []byte(`globalThis.onlySideEffect = true;`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = renderer.Render(bundle, nil)
	if err == nil || !strings.Contains(err.Error(), "did not define") {
		t.Fatalf("error = %v, want stale global rejection", err)
	}
}

func TestRendererStopWithInFlightRender(t *testing.T) {
	bundle := writeBundle(t, `export function render() { while (true) {} }`)
	renderer, err := NewRenderer(core.ModeProd, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	renderer.execTimeout = 200 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		_, err := renderer.Render(bundle, nil)
		done <- err
	}()
	time.Sleep(30 * time.Millisecond)

	stopDone := make(chan error, 1)
	go func() { stopDone <- renderer.Stop() }()
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop hung with a render in flight")
	}

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("in-flight render unexpectedly succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight render did not terminate after Stop")
	}
}

func TestRendererMapsJSExceptionsToStructuredError(t *testing.T) {
	bundle := writeBundle(t, `export function render() { throw new TypeError("boom"); }`)
	renderer, err := NewRenderer(core.ModeProd, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = renderer.Stop() }()

	_, err = renderer.Render(bundle, nil)
	var structured *core.StructuredError
	if !errors.As(err, &structured) {
		t.Fatalf("error = %T %v, want *core.StructuredError", err, err)
	}
	if structured.ErrorType != "Render Error" {
		t.Fatalf("error type = %q", structured.ErrorType)
	}
	if !strings.Contains(structured.Message, "boom") || !strings.Contains(structured.Message, "Failed to import component:") {
		t.Fatalf("message = %q", structured.Message)
	}
	if structured.Stack == "" {
		t.Fatal("stack is empty")
	}
}

func TestRendererDelegatesBuilds(t *testing.T) {
	builder := &fakeBuilder{}
	renderer, err := NewRenderer(core.ModeProd, 1, builder)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = renderer.Stop() }()

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
