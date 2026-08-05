package process

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func benchmarkRenderer(b *testing.B) (*Renderer, string) {
	b.Helper()
	if _, err := exec.LookPath("bun"); err != nil {
		b.Skip("bun is not available")
	}
	component := filepath.Join(b.TempDir(), "page.js")
	if err := os.WriteFile(component, []byte(`export async function render(props) {
  return { head: "<title>Bench</title>", html: "<main>Hello " + props.name + "</main>" };
}`), 0o644); err != nil {
		b.Fatal(err)
	}
	renderer, err := NewRenderer(core.ModeProd, RuntimeSource(core.ModeProd), "BIFROST_DEV=0")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = renderer.Stop() })
	if _, err := renderer.Render(component, map[string]any{"name": "World"}); err != nil {
		b.Fatal(err)
	}
	return renderer, component
}

func BenchmarkRendererLatency(b *testing.B) {
	renderer, component := benchmarkRenderer(b)
	latencies := make([]time.Duration, b.N)
	props := map[string]any{"name": "World"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		if _, err := renderer.Render(component, props); err != nil {
			b.Fatal(err)
		}
		latencies[i] = time.Since(start)
	}
	b.StopTimer()

	slices.Sort(latencies)
	if len(latencies) > 0 {
		b.ReportMetric(float64(latencies[len(latencies)/2].Microseconds()), "p50-us")
		b.ReportMetric(float64(latencies[(len(latencies)-1)*95/100].Microseconds()), "p95-us")
	}
}

func BenchmarkRendererParallel(b *testing.B) {
	renderer, component := benchmarkRenderer(b)
	props := map[string]any{"name": "World"}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := renderer.Render(component, props); err != nil {
				b.Error(err)
				return
			}
		}
	})
}

func BenchmarkRendererStartup(b *testing.B) {
	if _, err := exec.LookPath("bun"); err != nil {
		b.Skip("bun is not available")
	}
	source := RuntimeSource(core.ModeProd)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		renderer, err := NewRenderer(core.ModeProd, source, "BIFROST_DEV=0")
		if err != nil {
			b.Fatal(err)
		}
		if err := renderer.Stop(); err != nil {
			b.Fatal(err)
		}
	}
}
