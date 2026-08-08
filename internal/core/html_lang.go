package core

import (
	"regexp"
	"strings"
)

const DefaultHTMLLang = "en"

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

// ResolveHTMLDocumentAttrs resolves document attributes: the pre loader result
// wins, then the page config, then the app default.
func ResolveHTMLDocumentAttrs(appDefaultLang, pageLang, pageClass string, pre PreLoaderResult) (lang string, htmlClass string) {
	switch {
	case strings.TrimSpace(pre.Lang) != "":
		lang = SanitizeHTMLLang(pre.Lang)
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
	case strings.TrimSpace(pageClass) != "":
		htmlClass = SanitizeHTMLClass(pageClass)
	default:
		htmlClass = ""
	}

	return lang, htmlClass
}
