package process

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	moderncrenderer "github.com/3-lines-studio/bifrost/internal/adapters/modernc"
	quickjsrenderer "github.com/3-lines-studio/bifrost/internal/adapters/quickjs"
	sobekrenderer "github.com/3-lines-studio/bifrost/internal/adapters/sobek"
	"github.com/3-lines-studio/bifrost/internal/core"
)

func BenchmarkRuntimeRealPageStartupAndFirstRender(b *testing.B) {
	bundle := realPageBundle(b)
	props := map[string]any{"name": "Benchmark"}
	if info, err := os.Stat(bundle); err == nil {
		b.ReportMetric(float64(info.Size()), "bundle-B")
	}

	b.Run("Sobek", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			renderer, err := sobekrenderer.NewRenderer(core.ModeProd, 1, nil)
			if err != nil {
				b.Fatal(err)
			}
			page, err := renderer.Render(bundle, props)
			if stopErr := renderer.Stop(); err == nil {
				err = stopErr
			}
			if err != nil {
				b.Fatal(err)
			}
			if page.Body == "" || page.Head == "" {
				b.Fatal("renderer returned an empty page")
			}
		}
	})
	b.Run("QuickJS", func(b *testing.B) {
		if strings.Contains(bundle, "#") {
			b.Skip("current real-page artifact is a Sobek registry")
		}
		b.ReportAllocs()
		for range b.N {
			renderer, err := quickjsrenderer.NewRenderer(core.ModeProd, 1, nil)
			if err != nil {
				b.Fatal(err)
			}
			page, err := renderer.Render(bundle, props)
			if stopErr := renderer.Stop(); err == nil {
				err = stopErr
			}
			if err != nil {
				b.Fatal(err)
			}
			if page.Body == "" || page.Head == "" {
				b.Fatal("renderer returned an empty page")
			}
		}
	})
	b.Run("Modernc", func(b *testing.B) {
		if strings.Contains(bundle, "#") {
			b.Skip("current real-page artifact is a Sobek registry")
		}
		b.ReportAllocs()
		for range b.N {
			renderer, err := moderncrenderer.NewRenderer(core.ModeProd, 1, nil)
			if err != nil {
				b.Fatal(err)
			}
			page, err := renderer.Render(bundle, props)
			if stopErr := renderer.Stop(); err == nil {
				err = stopErr
			}
			if err != nil {
				b.Fatal(err)
			}
			if page.Body == "" || page.Head == "" {
				b.Fatal("renderer returned an empty page")
			}
		}
	})
}

func BenchmarkRuntimeRealPageSerial(b *testing.B) {
	bundle := realPageBundle(b)
	props := map[string]any{"name": "Benchmark"}

	b.Run("Bun", func(b *testing.B) {
		if strings.Contains(bundle, "#") {
			b.Skip("current real-page artifact is a Sobek registry")
		}
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
	b.Run("QuickJS", func(b *testing.B) {
		if strings.Contains(bundle, "#") {
			b.Skip("current real-page artifact is a Sobek registry")
		}
		renderer := benchmarkQuickJSRuntime(b, 1)
		assertBenchmarkRender(b, renderer, bundle, props)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if _, err := renderer.Render(bundle, props); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Modernc", func(b *testing.B) {
		if strings.Contains(bundle, "#") {
			b.Skip("current real-page artifact is a Sobek registry")
		}
		renderer := benchmarkModerncRuntime(b, 1)
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
		if strings.Contains(bundle, "#") {
			b.Skip("current real-page artifact is a Sobek registry")
		}
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

	workerCounts := []int{1, 2, 4, 6, 8, runtime.GOMAXPROCS(0)}
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
		b.Run("QuickJSWorkers"+strconv.Itoa(workers), func(b *testing.B) {
			if strings.Contains(bundle, "#") {
				b.Skip("current real-page artifact is a Sobek registry")
			}
			renderer := benchmarkQuickJSRuntime(b, workers)
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
		b.Run("ModerncWorkers"+strconv.Itoa(workers), func(b *testing.B) {
			if strings.Contains(bundle, "#") {
				b.Skip("current real-page artifact is a Sobek registry")
			}
			renderer := benchmarkModerncRuntime(b, workers)
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

func benchmarkModerncRuntime(b *testing.B, workers int) *moderncrenderer.Renderer {
	b.Helper()
	renderer, err := moderncrenderer.NewRenderer(core.ModeProd, workers, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = renderer.Stop() })
	return renderer
}

func benchmarkQuickJSRuntime(b *testing.B, workers int) *quickjsrenderer.Renderer {
	b.Helper()
	renderer, err := quickjsrenderer.NewRenderer(core.ModeProd, workers, nil)
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
	if configured := os.Getenv("BIFROST_BENCH_BUNDLE"); configured != "" {
		if _, err := os.Stat(configured); err != nil {
			tb.Fatalf("configured benchmark bundle is unavailable: %v", err)
		}
		return configured
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		tb.Fatal(err)
	}
	bifrostDir := filepath.Join(repoRoot, "example", "cmd", "full", ".bifrost")
	manifestData, err := os.ReadFile(filepath.Join(bifrostDir, "manifest.json"))
	if err == nil {
		manifest, parseErr := core.ParseManifest(manifestData)
		if parseErr == nil {
			if entry, ok := manifest.Entries["pages-home-entry"]; ok && entry.SSR != "" {
				parts := strings.SplitN(entry.SSR, "#", 2)
				bundlePath := filepath.Join(bifrostDir, filepath.FromSlash(strings.TrimPrefix(parts[0], "/")))
				if _, statErr := os.Stat(bundlePath); statErr == nil {
					if len(parts) == 2 {
						bundlePath += "#" + parts[1]
					}
					return bundlePath
				}
			}
		}
	}
	path := filepath.Join(bifrostDir, "ssr", "pages-home-entry-ssr.js")
	if _, err := os.Stat(path); err != nil {
		tb.Skipf("real SSR benchmark bundle is unavailable; build the example first: %v", err)
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
