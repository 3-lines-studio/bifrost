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
		".hero{display:block}",
		[]string{"/dist/page.css"},
		nil,
		"en",
		"",
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
	if !strings.Contains(html, `<style data-bifrost-critical>.hero{display:block}</style>`) {
		t.Error("expected inline critical CSS in output")
	}
	if !strings.Contains(html, `<link rel="stylesheet" href="/dist/page.css" />`) {
		t.Error("expected stylesheet link in output")
	}
	if strings.Contains(html, `media="print"`) || strings.Contains(html, `onload="this.media='all'"`) {
		t.Error("did not expect deferred stylesheet loading")
	}
	if !strings.Contains(html, "<title>Test</title>") {
		t.Error("expected custom title in output")
	}
}

func TestRenderHTMLShell_MissingScript(t *testing.T) {
	_, err := RenderHTMLShell("", nil, "", "", "", nil, nil, "", "")
	if err == nil {
		t.Error("expected error for missing script src")
	}
}

func TestRenderHTMLShell_DefaultTitle(t *testing.T) {
	html, err := RenderHTMLShell("", nil, "/dist/page.js", "", "", nil, nil, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, "<title>Bifrost</title>") {
		t.Error("expected default Bifrost title")
	}
}

func TestRenderHTMLShell_CustomTitleSuppressesDefault(t *testing.T) {
	html, err := RenderHTMLShell("", nil, "/dist/page.js", "<title>My App</title>", "", nil, nil, "", "")
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

func TestRenderHTMLShell_CustomLang(t *testing.T) {
	html, err := RenderHTMLShell("", nil, "/dist/page.js", "", "", nil, nil, "fr-CA", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, `<html lang="fr-CA">`) {
		t.Error("expected fr-CA lang on html element")
	}
}

func TestRenderHTMLShell_InvalidLangFallsBack(t *testing.T) {
	html, err := RenderHTMLShell("", nil, "/dist/page.js", "", "", nil, nil, `en"><script`, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, `<html lang="en">`) {
		t.Error("expected sanitized fallback to en")
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
		nil,
		"",
		"",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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
		nil,
		[]string{"/dist/chunk-a.js", "/dist/chunk-b.js"},
		"en",
		"",
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
	html, err := RenderHTMLShell("", nil, "/dist/page.js", "", "", nil, nil, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, `{}`) {
		t.Error("expected empty JSON object for nil props")
	}
}

func TestRenderHTMLShell_CustomClass(t *testing.T) {
	html, err := RenderHTMLShell("", nil, "/dist/page.js", "", "", nil, nil, "en", "dark")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, `<html lang="en" class="dark">`) {
		t.Error("expected html class on document element")
	}
}

func TestRenderHTMLShell_ClassEscaped(t *testing.T) {
	html, err := RenderHTMLShell("", nil, "/dist/page.js", "", "", nil, nil, "en", `dark" onclick="alert(1)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, `class="dark&#34; onclick=&#34;alert(1)"`) {
		t.Error("expected escaped html class")
	}
}

func TestRenderStyleTags_StylesheetOnly(t *testing.T) {
	html := RenderStyleTags("", []string{"/dist/page.css"})
	if html != `<link rel="stylesheet" href="/dist/page.css" />` {
		t.Fatalf("unexpected output: %q", html)
	}
	if strings.Contains(html, "data-bifrost-critical") {
		t.Fatal("did not expect critical style tag")
	}
}

func TestRenderStyleTags_CriticalOnly(t *testing.T) {
	html := RenderStyleTags(".hero{display:block}", nil)
	if !strings.Contains(html, `data-bifrost-critical`) {
		t.Fatal("expected critical style tag")
	}
	if strings.Contains(html, `rel="stylesheet"`) {
		t.Fatal("did not expect stylesheet link")
	}
}

func TestRenderStyleTags_MultipleStylesheets(t *testing.T) {
	html := RenderStyleTags("", []string{"/dist/a.css", "/dist/b.css"})
	if !strings.Contains(html, `href="/dist/a.css"`) || !strings.Contains(html, `href="/dist/b.css"`) {
		t.Fatalf("expected both links: %q", html)
	}
}

func TestRenderHTMLShell_MultipleStylesheets(t *testing.T) {
	html, err := RenderHTMLShell(
		"<div>x</div>",
		nil,
		"/dist/page.js",
		"<title>T</title>",
		"",
		[]string{"/dist/first.css", "/dist/second.css"},
		nil,
		"en",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(html, `rel="stylesheet"`) != 2 {
		t.Fatalf("expected 2 stylesheet links, got: %q", html)
	}
}
