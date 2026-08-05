package react

import (
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestSSREntryTemplate(t *testing.T) {
	tmpl := SSREntryTemplate()
	for _, want := range []string{"COMPONENT_PATH", "renderToString", "pageEl"} {
		if !strings.Contains(tmpl, want) {
			t.Fatalf("SSR template does not contain %q", want)
		}
	}
}

func TestClientEntryTemplates(t *testing.T) {
	if tmpl := ClientEntryTemplate(core.ModeSSR); !strings.Contains(tmpl, "hydrateRoot") {
		t.Fatal("hydration template does not contain hydrateRoot")
	}
	if tmpl := ClientEntryTemplate(core.ModeClientOnly); !strings.Contains(tmpl, "createRoot") {
		t.Fatal("client-only template does not contain createRoot")
	}
}

func TestRuntimeSource(t *testing.T) {
	dev := RuntimeSource(core.ModeDev)
	if !strings.Contains(dev, "Bun.serve") || !strings.Contains(dev, "react-compiler") {
		t.Fatal("development runtime is missing React support")
	}
	if prod := RuntimeSource(core.ModeProd); strings.Contains(prod, "bun-plugin-tailwind") {
		t.Fatal("production runtime contains the development Tailwind plugin")
	}
}
