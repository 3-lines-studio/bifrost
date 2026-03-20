package core

import (
	"strings"
	"testing"
)

func TestRenderHTMLShell_Basic(t *testing.T) {
	html, err := RenderHTMLShell(
		"<div>Hello</div>",
		map[string]any{"name": "World"},
		"/dist/page.js",
		"<title>Test</title>",
		"/dist/page.css",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, "<div>Hello</div>") {
		t.Error("expected body HTML in output")
	}
	if !strings.Contains(html, `"name":"World"`) {
		t.Error("expected props JSON in output")
	}
	if !strings.Contains(html, `src="/dist/page.js"`) {
		t.Error("expected script src in output")
	}
	head, _, ok := strings.Cut(html, "</head>")
	if !ok {
		t.Fatal("expected </head>")
	}
	if !strings.Contains(head, `rel="modulepreload"`) || !strings.Contains(head, `href="/dist/page.js"`) {
		t.Error("expected modulepreload for entry script in head")
	}
	if strings.Contains(head, `href="/dist/chunk`) {
		t.Error("did not expect chunk modulepreload without chunks")
	}
	if !strings.Contains(html, `href="/dist/page.css"`) {
		t.Error("expected CSS href in output")
	}
	if !strings.Contains(html, `media="print"`) || !strings.Contains(html, `onload="this.media='all'"`) {
		t.Error("expected non-blocking stylesheet attributes")
	}
	if !strings.Contains(html, "<noscript><link rel=\"stylesheet\"") {
		t.Error("expected noscript stylesheet fallback")
	}
	if !strings.Contains(html, "<title>Test</title>") {
		t.Error("expected custom title in output")
	}
}

func TestRenderHTMLShell_MissingScript(t *testing.T) {
	_, err := RenderHTMLShell("", nil, "", "", "", nil)
	if err == nil {
		t.Error("expected error for missing script src")
	}
}

func TestRenderHTMLShell_DefaultTitle(t *testing.T) {
	html, err := RenderHTMLShell("", nil, "/dist/page.js", "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, "<title>Bifrost</title>") {
		t.Error("expected default Bifrost title")
	}
}

func TestRenderHTMLShell_CustomTitleSuppressesDefault(t *testing.T) {
	html, err := RenderHTMLShell("", nil, "/dist/page.js", "<title>My App</title>", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Count(html, "<title>") != 1 {
		t.Error("expected exactly one title tag")
	}
	if !strings.Contains(html, "<title>My App</title>") {
		t.Error("expected custom title")
	}
}

func TestRenderHTMLShell_ScriptBreakoutEscaped(t *testing.T) {
	html, err := RenderHTMLShell(
		"",
		map[string]any{"xss": "</script><script>alert(1)</script>"},
		"/dist/page.js",
		"",
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Go's json.Marshal escapes < and > to \u003c and \u003e, so the literal
	// </script> sequence should never appear in the props JSON block.
	propsStart := strings.Index(html, `id="__BIFROST_PROPS__"`)
	if propsStart == -1 {
		t.Fatal("could not find props script tag")
	}
	propsSection := html[propsStart:]
	before, _, ok := strings.Cut(propsSection, "</script>")
	if !ok {
		t.Fatal("could not find closing </script> for props")
	}
	propsContent := before
	if strings.Contains(propsContent, "<script>") {
		t.Error("XSS: unexpected <script> inside props JSON block")
	}
}

func TestRenderHTMLShell_WithChunks(t *testing.T) {
	html, err := RenderHTMLShell(
		"",
		nil,
		"/dist/page.js",
		"",
		"",
		[]string{"/dist/chunk-a.js", "/dist/chunk-b.js"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, `src="/dist/chunk-a.js"`) {
		t.Error("expected chunk-a script in output")
	}
	if !strings.Contains(html, `src="/dist/chunk-b.js"`) {
		t.Error("expected chunk-b script in output")
	}
	head, _, ok := strings.Cut(html, "</head>")
	if !ok {
		t.Fatal("expected </head>")
	}
	if !strings.Contains(head, `modulepreload" href="/dist/chunk-a.js"`) {
		t.Error("expected modulepreload for chunk-a in head")
	}
	if !strings.Contains(head, `modulepreload" href="/dist/chunk-b.js"`) {
		t.Error("expected modulepreload for chunk-b in head")
	}
	if !strings.Contains(head, `modulepreload" href="/dist/page.js"`) {
		t.Error("expected modulepreload for entry in head")
	}
}

func TestRenderHTMLShell_NilProps(t *testing.T) {
	html, err := RenderHTMLShell("", nil, "/dist/page.js", "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, `{}`) {
		t.Error("expected empty JSON object for nil props")
	}
}
