package core

import (
	"regexp"
	"strings"
)

const DefaultHTMLLang = "en"

const PropHTMLLang = "__bifrost_html_lang"

const PropHTMLClass = "__bifrost_html_class"

var htmlLangTagPattern = regexp.MustCompile(`^[a-zA-Z]{2,8}(-[a-zA-Z0-9]{1,8})*$`)

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

func SanitizeHTMLClass(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func ResolveHTMLDocumentAttrs(appDefaultLang, pageLang, pageClass string, props map[string]any) (lang string, htmlClass string, propsForReact map[string]any) {
	var fromLoaderLang string
	var fromLoaderClass string
	if props != nil {
		_, hasLang := props[PropHTMLLang]
		_, hasClass := props[PropHTMLClass]
		if rawLang, ok := props[PropHTMLLang].(string); ok {
			fromLoaderLang = rawLang
		}
		if rawClass, ok := props[PropHTMLClass].(string); ok {
			fromLoaderClass = rawClass
		}
		if !hasLang && !hasClass {
			propsForReact = props
		} else {
			propsForReact = make(map[string]any, len(props))
			for k, v := range props {
				switch k {
				case PropHTMLLang, PropHTMLClass:
					continue
				}
				propsForReact[k] = v
			}
		}
	}

	switch {
	case strings.TrimSpace(fromLoaderLang) != "":
		lang = SanitizeHTMLLang(fromLoaderLang)
	case strings.TrimSpace(pageLang) != "":
		lang = SanitizeHTMLLang(pageLang)
	case strings.TrimSpace(appDefaultLang) != "":
		lang = SanitizeHTMLLang(appDefaultLang)
	default:
		lang = DefaultHTMLLang
	}

	switch {
	case strings.TrimSpace(fromLoaderClass) != "":
		htmlClass = SanitizeHTMLClass(fromLoaderClass)
	case strings.TrimSpace(pageClass) != "":
		htmlClass = SanitizeHTMLClass(pageClass)
	default:
		htmlClass = ""
	}

	return lang, htmlClass, propsForReact
}

func ResolveHTMLLang(appDefault, pageLang string, props map[string]any) (lang string, propsForReact map[string]any) {
	lang, _, propsForReact = ResolveHTMLDocumentAttrs(appDefault, pageLang, "", props)
	return lang, propsForReact
}
