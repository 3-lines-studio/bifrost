package core

import (
	"testing"
)

func TestResolveHTMLLang_Precedence(t *testing.T) {
	props := map[string]any{
		PropHTMLLang: "de",
		"title":      "x",
	}
	lang, out := ResolveHTMLLang("en", "fr", props)
	if lang != "de" {
		t.Fatalf("loader wins: got %q", lang)
	}
	outMap, ok := out.(map[string]any)
	if !ok {
		t.Fatal("expected map output")
	}
	if _, ok := outMap[PropHTMLLang]; ok {
		t.Fatal("reserved key should be stripped")
	}
	if outMap["title"] != "x" {
		t.Fatal("other props preserved")
	}

	lang2, out2 := ResolveHTMLLang("en", "fr", map[string]any{"k": 1})
	if lang2 != "fr" {
		t.Fatalf("page option: got %q", lang2)
	}
	out2Map, ok := out2.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", out2)
	}
	if len(out2Map) != 1 {
		t.Fatalf("expected one prop, got %v", out2)
	}

	lang3, _ := ResolveHTMLLang("it", "", nil)
	if lang3 != "it" {
		t.Fatalf("app default: got %q", lang3)
	}

	lang4, _ := ResolveHTMLLang("", "", nil)
	if lang4 != DefaultHTMLLang {
		t.Fatalf("builtin default: got %q", lang4)
	}
}

func TestResolveHTMLLang_NilProps(t *testing.T) {
	lang, out := ResolveHTMLLang("", "es", nil)
	if lang != "es" {
		t.Fatalf("got %q", lang)
	}
	if out != nil {
		t.Fatal("expected nil propsForReact when props nil")
	}
}

func TestResolveHTMLDocumentAttrs_StructWithReservedKeys(t *testing.T) {
	type myProps struct {
		Title            string `json:"title"`
		BifrostHTMLLang  string `json:"__bifrost_html_lang"`
		BifrostHTMLClass string `json:"__bifrost_html_class"`
	}
	lang, class, out := ResolveHTMLDocumentAttrs("en", "fr", "light", myProps{
		Title:            "x",
		BifrostHTMLLang:  "de",
		BifrostHTMLClass: "dark",
	})
	if lang != "de" {
		t.Fatalf("expected de from struct, got %q", lang)
	}
	if class != "dark" {
		t.Fatalf("expected dark from struct, got %q", class)
	}
	outMap, ok := out.(map[string]any)
	if !ok {
		t.Fatal("expected map output when reserved keys stripped from struct")
	}
	if _, ok := outMap["__bifrost_html_lang"]; ok {
		t.Fatal("reserved lang key should be stripped")
	}
	if outMap["title"] != "x" {
		t.Fatal("other props preserved")
	}
}

func TestResolveHTMLDocumentAttrs_StructWithoutReservedKeys(t *testing.T) {
	type myProps struct {
		Title string `json:"title"`
		Name  string `json:"name"`
	}
	original := myProps{Title: "x", Name: "y"}
	lang, class, out := ResolveHTMLDocumentAttrs("en", "fr", "", original)
	if lang != "fr" {
		t.Fatalf("expected page lang, got %q", lang)
	}
	if class != "" {
		t.Fatalf("expected empty class, got %q", class)
	}
	// Struct without reserved keys passes through as-is
	if out != original {
		t.Fatalf("expected original struct returned, got %v", out)
	}
}

func TestResolveHTMLDocumentAttrs_StructWithClass(t *testing.T) {
	type myProps struct {
		Title            string `json:"title"`
		BifrostHTMLClass string `json:"__bifrost_html_class"`
	}
	lang, class, out := ResolveHTMLDocumentAttrs("en", "", "", myProps{
		Title:            "x",
		BifrostHTMLClass: "dark contrast",
	})
	if lang != "en" {
		t.Fatalf("expected en, got %q", lang)
	}
	if class != "dark contrast" {
		t.Fatalf("expected class from struct, got %q", class)
	}
	outMap, ok := out.(map[string]any)
	if !ok {
		t.Fatal("expected map output when reserved keys stripped")
	}
	if _, ok := outMap["__bifrost_html_class"]; ok {
		t.Fatal("reserved class key should be stripped")
	}
}

func TestResolveHTMLDocumentAttrs_ClassPrecedence(t *testing.T) {
	props := map[string]any{
		PropHTMLClass: "dark  contrast",
		"title":       "x",
	}
	lang, class, out := ResolveHTMLDocumentAttrs("en", "fr", "light", props)
	if lang != "fr" {
		t.Fatalf("expected page lang, got %q", lang)
	}
	if class != "dark contrast" {
		t.Fatalf("expected loader class, got %q", class)
	}
	outMap, ok := out.(map[string]any)
	if !ok {
		t.Fatal("expected map output")
	}
	if _, ok := outMap[PropHTMLClass]; ok {
		t.Fatal("reserved html class key should be stripped")
	}
	if outMap["title"] != "x" {
		t.Fatal("other props preserved")
	}
}

func TestResolveHTMLDocumentAttrs_PageClassFallback(t *testing.T) {
	props := map[string]any{"k": 1}
	lang, class, out := ResolveHTMLDocumentAttrs("", "es", " dark ", props)
	if lang != "es" {
		t.Fatalf("expected page lang, got %q", lang)
	}
	if class != "dark" {
		t.Fatalf("expected sanitized page class, got %q", class)
	}
	outMap, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", out)
	}
	if len(outMap) != 1 {
		t.Fatalf("expected one prop, got %v", out)
	}
	outMap["copy_check"] = true
	if props["copy_check"] != true {
		t.Fatal("expected props map to be reused when no reserved keys are present")
	}
}

func TestResolveHTMLDocumentAttrs_NilProps(t *testing.T) {
	lang, class, out := ResolveHTMLDocumentAttrs("", "es", "dark", nil)
	if lang != "es" {
		t.Fatalf("got lang %q", lang)
	}
	if class != "dark" {
		t.Fatalf("got class %q", class)
	}
	if out != nil {
		t.Fatal("expected nil propsForReact when props nil")
	}
}

func TestResolveHTMLDocumentAttrs_ReservedKeysForceCopy(t *testing.T) {
	props := map[string]any{
		PropHTMLLang:  "pt-BR",
		PropHTMLClass: "contrast",
		"k":           1,
	}
	lang, class, out := ResolveHTMLDocumentAttrs("en", "es", "dark", props)
	if lang != "pt-BR" {
		t.Fatalf("expected loader lang, got %q", lang)
	}
	if class != "contrast" {
		t.Fatalf("expected loader class, got %q", class)
	}
	outMap, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", out)
	}
	outMap["copy_check"] = true
	if _, ok := props["copy_check"]; ok {
		t.Fatal("expected props map to be copied when reserved keys are stripped")
	}
	if _, ok := outMap[PropHTMLLang]; ok {
		t.Fatal("reserved lang key should be stripped")
	}
	if _, ok := outMap[PropHTMLClass]; ok {
		t.Fatal("reserved class key should be stripped")
	}
}

func TestResolveHTMLDocumentAttrs_StructOnlyReservedKeys(t *testing.T) {
	type myProps struct {
		BifrostHTMLLang  string `json:"__bifrost_html_lang"`
		BifrostHTMLClass string `json:"__bifrost_html_class"`
	}
	lang, class, out := ResolveHTMLDocumentAttrs("en", "", "", myProps{
		BifrostHTMLLang:  "es",
		BifrostHTMLClass: "dark",
	})
	if lang != "es" {
		t.Fatalf("expected es, got %q", lang)
	}
	if class != "dark" {
		t.Fatalf("expected dark, got %q", class)
	}
	outMap, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %T", out)
	}
	if len(outMap) != 0 {
		t.Fatalf("expected empty cleaned map, got %v", outMap)
	}
}

func BenchmarkResolveHTMLDocumentAttrs_NilProps(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _ = ResolveHTMLDocumentAttrs("en", "es", "dark", nil)
	}
}

func BenchmarkResolveHTMLDocumentAttrs_NoReservedKeys(b *testing.B) {
	b.ReportAllocs()
	props := map[string]any{"k": 1, "title": "x"}
	for i := 0; i < b.N; i++ {
		_, _, _ = ResolveHTMLDocumentAttrs("en", "es", "dark", props)
	}
}

func BenchmarkResolveHTMLDocumentAttrs_WithReservedKeys(b *testing.B) {
	b.ReportAllocs()
	props := map[string]any{
		PropHTMLLang:  "de",
		PropHTMLClass: "contrast",
		"k":           1,
		"title":       "x",
	}
	for i := 0; i < b.N; i++ {
		_, _, _ = ResolveHTMLDocumentAttrs("en", "es", "dark", props)
	}
}
