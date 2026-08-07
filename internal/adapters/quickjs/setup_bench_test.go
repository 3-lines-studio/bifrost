package quickjs

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var (
	exampleBundleOnce sync.Once
	exampleBundlePath string
)

// exampleHomeBundle returns a bundle path for benchmarks, writing a small
// SSR bundle to a temp file on first use.
func exampleHomeBundle(b *testing.B) (string, bool) {
	exampleBundleOnce.Do(func() {
		dir, err := os.MkdirTemp("", "bifrost-bench-*")
		if err != nil {
			return
		}
		path := filepath.Join(dir, "pages-home-entry-ssr.js")
		content := []byte(`export async function render(props) { return { head: "<title>" + props.name + "</title>", html: "<main>Hello " + props.name + "</main>" }; }`)
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return
		}
		exampleBundlePath = path
	})
	if exampleBundlePath == "" {
		return "", true
	}
	return exampleBundlePath, false
}
