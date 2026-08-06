package quickjs

import (
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

// BenchmarkWorkerInit isolates the per-worker setup cost (runtime, context,
// shims, interrupt handler) from bundle evaluation and rendering.
func BenchmarkWorkerInit(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		renderer, err := NewRenderer(core.ModeProd, 1, nil)
		if err != nil {
			b.Fatal(err)
		}
		if err := renderer.Stop(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWorkerInitAndIIFEEval adds evaluating the prebuilt IIFE bundle.
func BenchmarkWorkerInitAndIIFEEval(b *testing.B) {
	bundle, skip := exampleHomeBundle(b)
	if skip {
		b.Skip("example SSR bundle unavailable")
	}
	props := map[string]any{"name": "Benchmark"}
	b.ReportAllocs()
	for range b.N {
		renderer, err := NewRenderer(core.ModeProd, 1, nil)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := renderer.Render(bundle, props); err != nil {
			b.Fatal(err)
		}
		if err := renderer.Stop(); err != nil {
			b.Fatal(err)
		}
	}
}
