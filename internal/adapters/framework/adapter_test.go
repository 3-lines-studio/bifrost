package framework

import (
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestReactAdapterName(t *testing.T) {
	a := NewReactAdapter()
	if a.Name() != "react" {
		t.Fatalf("expected react, got %s", a.Name())
	}
}

func TestReactAdapterFileExtension(t *testing.T) {
	a := NewReactAdapter()
	if a.FileExtension() != ".tsx" {
		t.Fatalf("expected .tsx, got %s", a.FileExtension())
	}
}

func TestReactAdapterEntryFileExtension(t *testing.T) {
	a := NewReactAdapter()
	if a.EntryFileExtension() != ".tsx" {
		t.Fatalf("expected .tsx, got %s", a.EntryFileExtension())
	}
}

func TestReactAdapterSSREntryTemplate(t *testing.T) {
	a := NewReactAdapter()
	tmpl := a.SSREntryTemplate()
	if !strings.Contains(tmpl, "COMPONENT_PATH") {
		t.Fatal("expected COMPONENT_PATH placeholder")
	}
	if !strings.Contains(tmpl, "renderToString") {
		t.Fatal("expected renderToString in SSR template")
	}
	if !strings.Contains(tmpl, "pageEl") {
		t.Fatal("expected BIFROST_SSR_PAGE_WRAP replaced with pageEl")
	}
}

func TestReactAdapterClientEntryTemplates(t *testing.T) {
	a := NewReactAdapter()

	hydration := a.ClientEntryTemplate(core.ModeSSR)
	if !strings.Contains(hydration, "hydrateRoot") {
		t.Fatal("expected hydrateRoot in hydration template")
	}

	clientOnly := a.ClientEntryTemplate(core.ModeClientOnly)
	if !strings.Contains(clientOnly, "createRoot") {
		t.Fatal("expected createRoot in client-only template")
	}
}

func TestReactAdapterBuildPlugins(t *testing.T) {
	a := NewReactAdapter()
	plugins := a.BuildPlugins()
	if len(plugins) != 1 || plugins[0] != "bun-plugin-tailwind" {
		t.Fatalf("expected bun-plugin-tailwind, got %v", plugins)
	}
}

func TestReactAdapterRuntimeImports(t *testing.T) {
	a := NewReactAdapter()
	imports := a.RuntimeImports()
	found := make(map[string]bool)
	for _, imp := range imports {
		found[imp] = true
	}
	for _, want := range []string{"react", "react-dom/server", "react-dom/client"} {
		if !found[want] {
			t.Fatalf("expected %s in runtime imports", want)
		}
	}
}

func TestSvelteAdapterName(t *testing.T) {
	a := NewSvelteAdapter()
	if a.Name() != "svelte" {
		t.Fatalf("expected svelte, got %s", a.Name())
	}
}

func TestSvelteAdapterFileExtension(t *testing.T) {
	a := NewSvelteAdapter()
	if a.FileExtension() != ".svelte" {
		t.Fatalf("expected .svelte, got %s", a.FileExtension())
	}
}

func TestSvelteAdapterEntryFileExtension(t *testing.T) {
	a := NewSvelteAdapter()
	if a.EntryFileExtension() != ".ts" {
		t.Fatalf("expected .ts, got %s", a.EntryFileExtension())
	}
}

func TestSvelteAdapterSSREntryTemplate(t *testing.T) {
	a := NewSvelteAdapter()
	tmpl := a.SSREntryTemplate()
	if !strings.Contains(tmpl, "COMPONENT_PATH") {
		t.Fatal("expected COMPONENT_PATH placeholder")
	}
	if !strings.Contains(tmpl, "svelte/server") {
		t.Fatal("expected svelte/server import")
	}
	if !strings.Contains(tmpl, "export async function render") {
		t.Fatal("expected render export")
	}
}

func TestSvelteAdapterClientEntryTemplates(t *testing.T) {
	a := NewSvelteAdapter()

	hydration := a.ClientEntryTemplate(core.ModeSSR)
	if !strings.Contains(hydration, "hydrate") {
		t.Fatal("expected hydrate in hydration template")
	}
	if !strings.Contains(hydration, "__BIFROST_PROPS__") {
		t.Fatal("expected props hydration in hydration template")
	}

	clientOnly := a.ClientEntryTemplate(core.ModeClientOnly)
	if !strings.Contains(clientOnly, "mount") {
		t.Fatal("expected mount in client-only template")
	}
	if !strings.Contains(clientOnly, "target.innerHTML = \"\"") {
		t.Fatal("expected client-only template to clear existing SSR preview content before mounting")
	}
}

func TestSvelteAdapterBuildPlugins(t *testing.T) {
	a := NewSvelteAdapter()
	plugins := a.BuildPlugins()
	if len(plugins) != 1 || plugins[0] != "bun-plugin-tailwind" {
		t.Fatalf("expected bun-plugin-tailwind, got %v", plugins)
	}
}

func TestSvelteAdapterRuntimeImports(t *testing.T) {
	a := NewSvelteAdapter()
	imports := a.RuntimeImports()
	found := make(map[string]bool)
	for _, imp := range imports {
		found[imp] = true
	}
	for _, want := range []string{"svelte/compiler", "svelte/server", "svelte"} {
		if !found[want] {
			t.Fatalf("expected %s in runtime imports", want)
		}
	}
}

func TestResolveAdapter(t *testing.T) {
	if a := ResolveAdapter(core.FrameworkReact); a.Name() != "react" {
		t.Fatal("expected react adapter")
	}
	if a := ResolveAdapter(core.FrameworkSvelte); a.Name() != "svelte" {
		t.Fatal("expected svelte adapter")
	}
	if a := ResolveAdapter(core.Framework(999)); a.Name() != "react" {
		t.Fatal("expected react adapter for unknown framework")
	}
}

func TestResolveAdapterForPath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"./pages/home.tsx", "react"},
		{"./pages/home.tsx?t=123", "react"},
		{"./components/header.svelte", "svelte"},
		{"/abs/path/page.jsx", "react"},
		{"unknown", "react"},
		{"file.svelte", "svelte"},
		{"file.svelte.ts", "svelte"},
		{"file.svelte.js", "svelte"},
		{"file.svelte.ts?t=123", "svelte"},
		{"./components/avatar/avatar-context.svelte.ts", "svelte"},
		{"", "react"},
	}
	for _, tt := range tests {
		a := ResolveAdapterForPath(tt.path)
		if a.Name() != tt.expected {
			t.Errorf("ResolveAdapterForPath(%q) = %s, want %s", tt.path, a.Name(), tt.expected)
		}
	}
}

func TestDefaultAdapter(t *testing.T) {
	a := DefaultAdapter()
	if a.Name() != "react" {
		t.Fatal("expected react as default adapter")
	}
}

func TestDevRendererSourceContainsPlugins(t *testing.T) {
	for _, fw := range []core.Framework{core.FrameworkReact, core.FrameworkSvelte} {
		a := ResolveAdapter(fw)
		src := a.DevRendererSource()
		if !strings.Contains(src, "Bun.serve") {
			t.Fatalf("%s dev source missing Bun.serve", a.Name())
		}
		if !strings.Contains(src, "/render") && !strings.Contains(src, "/build") {
			t.Fatal("missing route handlers")
		}
	}
}

func TestProdRendererSourceNoTailwindPlugin(t *testing.T) {
	for _, fw := range []core.Framework{core.FrameworkReact, core.FrameworkSvelte} {
		a := ResolveAdapter(fw)
		src := a.ProdRendererSource()
		if strings.Contains(src, "bun-plugin-tailwind") {
			t.Fatalf("%s prod source should not include tailwind plugin", a.Name())
		}
	}
}
