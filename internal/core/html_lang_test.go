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
	if _, ok := out[PropHTMLLang]; ok {
		t.Fatal("reserved key should be stripped")
	}
	if out["title"] != "x" {
		t.Fatal("other props preserved")
	}

	lang2, out2 := ResolveHTMLLang("en", "fr", map[string]any{"k": 1})
	if lang2 != "fr" {
		t.Fatalf("page option: got %q", lang2)
	}
	if len(out2) != 1 {
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
