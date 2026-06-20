package core

import (
	"encoding/json"
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

func stripReservedKeysFromMap(m map[string]any) (cleaned map[string]any, lang string, class string, hasReserved bool) {
	rawLang, hasLang := m[PropHTMLLang].(string)
	rawClass, hasClass := m[PropHTMLClass].(string)
	if !hasLang && !hasClass {
		return m, "", "", false
	}
	cleaned = make(map[string]any, len(m))
	for k, v := range m {
		switch k {
		case PropHTMLLang, PropHTMLClass:
			continue
		}
		cleaned[k] = v
	}
	return cleaned, rawLang, rawClass, true
}

func ResolveHTMLDocumentAttrs(appDefaultLang, pageLang, pageClass string, props any) (lang string, htmlClass string, propsForReact any) {
	var fromLoaderLang string
	var fromLoaderClass string

	if props != nil {
		if m, ok := props.(map[string]any); ok {
			cleaned, rawLang, rawClass, hasReserved := stripReservedKeysFromMap(m)
			fromLoaderLang = rawLang
			fromLoaderClass = rawClass
			if hasReserved {
				propsForReact = cleaned
			} else {
				propsForReact = props
			}
		} else {
			data, err := json.Marshal(props)
			if err == nil {
				var m map[string]any
				if err := json.Unmarshal(data, &m); err == nil {
					var cleaned map[string]any
					var hasReserved bool
					cleaned, fromLoaderLang, fromLoaderClass, hasReserved = stripReservedKeysFromMap(m)
					if hasReserved {
						propsForReact = cleaned
					} else {
						propsForReact = props
					}
				} else {
					propsForReact = props
				}
			} else {
				propsForReact = props
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

func ResolveHTMLLang(appDefault, pageLang string, props any) (lang string, propsForReact any) {
	lang, _, propsForReact = ResolveHTMLDocumentAttrs(appDefault, pageLang, "", props)
	return lang, propsForReact
}
