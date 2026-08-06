package quickjs

import (
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func BenchmarkRenderExamplePage(b *testing.B) {
	bundle, skip := exampleHomeBundle(b)
	if skip {
		b.Skip("example SSR bundle unavailable")
	}
	props := map[string]any{"name": "Benchmark"}

	b.Run("SerialWorkers1", func(b *testing.B) {
		renderer, err := NewRenderer(core.ModeProd, 1, nil)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { _ = renderer.Stop() })
		if _, err := renderer.Render(bundle, props); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if _, err := renderer.Render(bundle, props); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ParallelWorkers4", func(b *testing.B) {
		renderer, err := NewRenderer(core.ModeProd, 4, nil)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { _ = renderer.Stop() })
		if _, err := renderer.Render(bundle, props); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				if _, err := renderer.Render(bundle, props); err != nil {
					b.Error(err)
					return
				}
			}
		})
	})
}
