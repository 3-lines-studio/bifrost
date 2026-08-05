package process

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	sobekrenderer "github.com/3-lines-studio/bifrost/internal/adapters/sobek"
	"github.com/3-lines-studio/bifrost/internal/core"
)

func BenchmarkRuntimeRealPageSerial(b *testing.B) {
	bundle := realPageBundle(b)
	props := map[string]any{"name": "Benchmark"}

	b.Run("Bun", func(b *testing.B) {
		renderer := benchmarkBunRuntime(b)
		assertBenchmarkRender(b, renderer, bundle, props)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if _, err := renderer.Render(bundle, props); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Sobek", func(b *testing.B) {
		renderer := benchmarkSobekRuntime(b, 1)
		assertBenchmarkRender(b, renderer, bundle, props)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if _, err := renderer.Render(bundle, props); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkRuntimeRealPageParallel(b *testing.B) {
	bundle := realPageBundle(b)
	props := map[string]any{"name": "Benchmark"}

	b.Run("Bun", func(b *testing.B) {
		renderer := benchmarkBunRuntime(b)
		assertBenchmarkRender(b, renderer, bundle, props)
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

	workerCounts := []int{1, 2, 4, runtime.GOMAXPROCS(0)}
	seen := make(map[int]struct{}, len(workerCounts))
	for _, workers := range workerCounts {
		if _, ok := seen[workers]; ok {
			continue
		}
		seen[workers] = struct{}{}
		b.Run("SobekWorkers"+strconv.Itoa(workers), func(b *testing.B) {
			renderer := benchmarkSobekRuntime(b, workers)
			assertBenchmarkRender(b, renderer, bundle, props)
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
}

type benchmarkPageRenderer interface {
	Render(path string, props any) (core.RenderedPage, error)
	Stop() error
}

func benchmarkBunRuntime(b *testing.B) *Renderer {
	b.Helper()
	renderer, err := NewRenderer(core.ModeProd, RuntimeSource(core.ModeProd), "BIFROST_DEV=0")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = renderer.Stop() })
	return renderer
}

func benchmarkSobekRuntime(b *testing.B, workers int) *sobekrenderer.Renderer {
	b.Helper()
	renderer, err := sobekrenderer.NewRenderer(core.ModeProd, workers, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = renderer.Stop() })
	return renderer
}

func realPageBundle(tb testing.TB) string {
	tb.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		tb.Fatal(err)
	}
	path := filepath.Join(
		repoRoot,
		"example",
		"cmd",
		"full",
		".bifrost",
		"ssr",
		"pages-home-entry-ssr.js",
	)
	if _, err := os.Stat(path); err != nil {
		tb.Skipf("real SSR benchmark bundle is unavailable; run 'make check': %v", err)
	}
	return path
}

func assertBenchmarkRender(tb testing.TB, renderer benchmarkPageRenderer, bundle string, props any) {
	tb.Helper()
	page, err := renderer.Render(bundle, props)
	if err != nil {
		tb.Fatal(err)
	}
	if page.Body == "" || page.Head == "" {
		tb.Fatal("renderer returned an empty page")
	}
}
