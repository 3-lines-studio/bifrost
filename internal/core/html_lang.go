package core

import (
	"regexp"
	"strings"
)

// DefaultHTMLLang is used when no lang is configured.
const DefaultHTMLLang = "en"

// PropHTMLLang is reserved in props maps from loaders and static data loaders; it sets
// the document <html lang> and is not passed to the page component.
const PropHTMLLang = "__bifrost_html_lang"

var htmlLangTagPattern = regexp.MustCompile(`^[a-zA-Z]{2,8}(-[a-zA-Z0-9]{1,8})*$`)

// SanitizeHTMLLang returns a safe BCP-47-like language tag or DefaultHTMLLang.
func SanitizeHTMLLang(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return DefaultHTMLLang
	}
	if htmlLangTagPattern.MatchString(s) {
		return s
	}
	return DefaultHTMLLang
}

// ResolveHTMLLang picks the document language with precedence: loader prop → page option → app default → built-in default.
// It returns a shallow copy of props with PropHTMLLang removed when props is non-nil.
func ResolveHTMLLang(appDefault, pageLang string, props map[string]any) (lang string, propsForReact map[string]any) {
	var fromLoader string
	if props != nil {
		propsForReact = make(map[string]any, len(props))
		for k, v := range props {
			if k == PropHTMLLang {
				if s, ok := v.(string); ok {
					fromLoader = s
				}
				continue
			}
			propsForReact[k] = v
		}
	}

	switch {
	case strings.TrimSpace(fromLoader) != "":
		lang = SanitizeHTMLLang(fromLoader)
	case strings.TrimSpace(pageLang) != "":
		lang = SanitizeHTMLLang(pageLang)
	case strings.TrimSpace(appDefault) != "":
		lang = SanitizeHTMLLang(appDefault)
	default:
		lang = DefaultHTMLLang
	}
	return lang, propsForReact
}
