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

func isPropsMap(props any) bool {
	_, ok := props.(map[string]any)
	return ok
}

func jsonRoundTrip(props any) map[string]any {
	data, err := json.Marshal(props)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}

func ResolveHTMLDocumentAttrs(appDefaultLang, pageLang, pageClass string, props any) (lang string, htmlClass string, propsForReact any) {
	return ResolveHTMLDocumentAttrsWithPre(appDefaultLang, pageLang, pageClass, PreLoaderResult{}, props)
}

// ResolveHTMLDocumentAttrsWithPre resolves document attributes with a pre loader
// result taking precedence over the reserved props keys, the page config, and
// the app default.
func ResolveHTMLDocumentAttrsWithPre(appDefaultLang, pageLang, pageClass string, pre PreLoaderResult, props any) (lang string, htmlClass string, propsForReact any) {
	var fromLoaderLang string
	var fromLoaderClass string

	if props != nil {
		switch {
		case isPropsMap(props):
			m := props.(map[string]any)
			cleaned, rawLang, rawClass, hasReserved := stripReservedKeysFromMap(m)
			fromLoaderLang = rawLang
			fromLoaderClass = rawClass
			if hasReserved {
				propsForReact = cleaned
			} else {
				propsForReact = props
			}
		default:
			// Use encoding/json for typed maps and structs so tags and MarshalJSON
			// define the same props shape sent to React.
			cleaned, rawLang, rawClass, hasReserved := stripReservedKeysFromMap(jsonRoundTrip(props))
			if hasReserved {
				fromLoaderLang = rawLang
				fromLoaderClass = rawClass
				propsForReact = cleaned
			} else {
				propsForReact = props
			}
		}
	}

	switch {
	case strings.TrimSpace(pre.Lang) != "":
		lang = SanitizeHTMLLang(pre.Lang)
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
	case strings.TrimSpace(pre.Class) != "":
		htmlClass = SanitizeHTMLClass(pre.Class)
	case strings.TrimSpace(fromLoaderClass) != "":
		htmlClass = SanitizeHTMLClass(fromLoaderClass)
	case strings.TrimSpace(pageClass) != "":
		htmlClass = SanitizeHTMLClass(pageClass)
	default:
		htmlClass = ""
	}

	return lang, htmlClass, propsForReact
}
