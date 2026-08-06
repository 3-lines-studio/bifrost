package core

import "strings"

const (
	JSRuntimeSobek   = "sobek"
	JSRuntimeBun     = "bun"
	JSRuntimeQuickJS = "quickjs"
)

// NormalizeJSRuntime returns the selected JavaScript backend. QuickJS is the
// default; Bun must be selected explicitly, and Sobek is available with an
// explicit value for projects that need a pure-Go build.
func NormalizeJSRuntime(value string) string {
	normalized := strings.TrimSpace(value)
	if strings.EqualFold(normalized, JSRuntimeBun) {
		return JSRuntimeBun
	}
	if strings.EqualFold(normalized, JSRuntimeSobek) {
		return JSRuntimeSobek
	}
	return JSRuntimeQuickJS
}
