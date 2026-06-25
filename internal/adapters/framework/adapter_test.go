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

func TestResolveAdapter(t *testing.T) {
	if a := ResolveAdapter(core.FrameworkReact); a.Name() != "react" {
		t.Fatal("expected react adapter")
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
		{"/abs/path/page.jsx", "react"},
		{"unknown", "react"},
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
	for _, fw := range []core.Framework{core.FrameworkReact} {
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
	for _, fw := range []core.Framework{core.FrameworkReact} {
		a := ResolveAdapter(fw)
		src := a.ProdRendererSource()
		if strings.Contains(src, "bun-plugin-tailwind") {
			t.Fatalf("%s prod source should not include tailwind plugin", a.Name())
		}
	}
}
