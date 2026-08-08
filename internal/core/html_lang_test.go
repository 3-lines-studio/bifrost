package core

import "testing"

func TestResolveHTMLDocumentAttrs_LangPrecedence(t *testing.T) {
	cases := []struct {
		name       string
		appDefault string
		pageLang   string
		pre        PreLoaderResult
		want       string
	}{
		{"pre loader wins", "en", "fr", PreLoaderResult{Lang: "pt"}, "pt"},
		{"page config over app default", "en", "fr", PreLoaderResult{}, "fr"},
		{"app default", "en", "", PreLoaderResult{}, "en"},
		{"default fallback", "", "", PreLoaderResult{}, "en"},
		{"invalid pre lang sanitized", "en", "fr", PreLoaderResult{Lang: "nope!"}, "en"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lang, _ := ResolveHTMLDocumentAttrs(c.appDefault, c.pageLang, "", c.pre)
			if lang != c.want {
				t.Errorf("lang = %q, want %q", lang, c.want)
			}
		})
	}
}

func TestResolveHTMLDocumentAttrs_ClassPrecedence(t *testing.T) {
	cases := []struct {
		name      string
		pageClass string
		pre       PreLoaderResult
		want      string
	}{
		{"pre loader wins", "light", PreLoaderResult{Class: "dark"}, "dark"},
		{"page config", "light", PreLoaderResult{}, "light"},
		{"empty default", "", PreLoaderResult{}, ""},
		{"whitespace joined", "dark  contrast", PreLoaderResult{}, "dark contrast"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, class := ResolveHTMLDocumentAttrs("en", "", c.pageClass, c.pre)
			if class != c.want {
				t.Errorf("class = %q, want %q", class, c.want)
			}
		})
	}
}

func TestSanitizeHTMLLang(t *testing.T) {
	cases := []struct{ in, want string }{
		{"en", "en"},
		{"pt-BR", "pt-BR"},
		{"en_US", "en"},
		{"nope!", "en"},
		{"  es  ", "es"},
		{"", "en"},
	}
	for _, c := range cases {
		if got := SanitizeHTMLLang(c.in); got != c.want {
			t.Errorf("SanitizeHTMLLang(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
