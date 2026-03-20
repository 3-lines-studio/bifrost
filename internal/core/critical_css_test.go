package core

import (
	"strings"
	"testing"
)

func TestExtractCriticalCSS_MatchingSelectorsOnly(t *testing.T) {
	html := `<body><div class="hero"><span id="cta">Hi</span></div></body>`
	css := `
		:root{--bg:white;}
		body{margin:0;}
		.hero{display:grid;}
		#cta{color:red;}
		.missing{display:none;}
	`

	critical := ExtractCriticalCSS(html, css, DefaultCriticalCSSMaxBytes)

	if !strings.Contains(critical, ":root{--bg:white;}") {
		t.Fatal("expected :root variables in critical CSS")
	}
	if !strings.Contains(critical, "body{margin:0;}") {
		t.Fatal("expected body rule in critical CSS")
	}
	if !strings.Contains(critical, ".hero{display:grid;}") {
		t.Fatal("expected matching class rule in critical CSS")
	}
	if !strings.Contains(critical, "#cta{color:red;}") {
		t.Fatal("expected matching id rule in critical CSS")
	}
	if strings.Contains(critical, ".missing{display:none;}") {
		t.Fatal("did not expect unrelated selector in critical CSS")
	}
}

func TestExtractCriticalCSS_KeepsNestedMediaAndKeyframes(t *testing.T) {
	html := `<body><div class="hero">Hi</div></body>`
	css := `
		@media (min-width: 768px) {.hero{animation:fade-in 1s ease;}}
		@keyframes fade-in {from{opacity:0;}to{opacity:1;}}
		@keyframes unused {from{opacity:0;}to{opacity:1;}}
	`

	critical := ExtractCriticalCSS(html, css, DefaultCriticalCSSMaxBytes)

	if !strings.Contains(critical, "@media (min-width: 768px)") || !strings.Contains(critical, ".hero{animation:fade-in 1s ease;}") {
		t.Fatal("expected matching nested media rule in critical CSS")
	}
	if !strings.Contains(critical, "@keyframes fade-in {from{opacity:0;}to{opacity:1;}}") {
		t.Fatal("expected referenced keyframes in critical CSS")
	}
	if strings.Contains(critical, "@keyframes unused") {
		t.Fatal("did not expect unused keyframes in critical CSS")
	}
}

func TestExtractCriticalCSS_RespectsSizeCap(t *testing.T) {
	html := `<body><div class="hero">Hi</div></body>`
	css := `.hero{padding:1rem;}`

	critical := ExtractCriticalCSS(html, css, 4)
	if critical != "" {
		t.Fatal("expected extraction to skip oversized critical CSS")
	}
}
