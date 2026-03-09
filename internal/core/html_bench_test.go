package core

import "testing"

func BenchmarkRenderHTMLShell_Minimal(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = RenderHTMLShell("", nil, "/dist/page.js", "", "", nil)
	}
}

func BenchmarkRenderHTMLShell_WithBody(b *testing.B) {
	b.ReportAllocs()
	body := "<div><h1>Hello World</h1><p>Some content here</p></div>"
	props := map[string]any{"name": "World", "count": 42}
	for i := 0; i < b.N; i++ {
		_, _ = RenderHTMLShell(body, props, "/dist/page.js", "<title>Test</title>", "/dist/page.css", nil)
	}
}

func BenchmarkRenderHTMLShell_WithChunks(b *testing.B) {
	b.ReportAllocs()
	chunks := []string{"/dist/chunk-a.js", "/dist/chunk-b.js", "/dist/chunk-c.js"}
	for i := 0; i < b.N; i++ {
		_, _ = RenderHTMLShell("<div>content</div>", map[string]any{"x": 1}, "/dist/page.js", "", "/dist/page.css", chunks)
	}
}

func BenchmarkRenderHTMLShell_LargeProps(b *testing.B) {
	b.ReportAllocs()
	props := make(map[string]any, 50)
	for i := range 50 {
		props["key_"+string(rune('a'+i%26))] = "value_" + string(rune('a'+i%26))
	}
	for i := 0; i < b.N; i++ {
		_, _ = RenderHTMLShell("<div>body</div>", props, "/dist/page.js", "<title>T</title>", "/dist/page.css", nil)
	}
}
