package bifrost

import (
	"fmt"
	"testing"
)

func BenchmarkModelStartup(b *testing.B) {
	for _, count := range []int{100, 1_000} {
		routes := make([]Route, count)
		for i := range count {
			routes[i] = Client(fmt.Sprintf("/page/%d", i), "pages/shared.tsx")
		}
		config := Config{Routes: routes}
		b.Run(fmt.Sprintf("routes_%d", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := newApp(config); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
