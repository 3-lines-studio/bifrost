package core

import "strings"

const (
	JSRuntimeSobek   = "sobek"
	JSRuntimeBun     = "bun"
	JSRuntimeQuickJS = "quickjs"
	JSRuntimeModernc = "modernc"
)

// NormalizeJSRuntime returns the selected JavaScript backend. QuickJS is the
// default; Bun must be selected explicitly, and Sobek and the pure-Go
// modernc port are available with explicit values.
func NormalizeJSRuntime(value string) string {
	normalized := strings.TrimSpace(value)
	if strings.EqualFold(normalized, JSRuntimeBun) {
		return JSRuntimeBun
	}
	if strings.EqualFold(normalized, JSRuntimeSobek) {
		return JSRuntimeSobek
	}
	if strings.EqualFold(normalized, JSRuntimeModernc) {
		return JSRuntimeModernc
	}
	return JSRuntimeQuickJS
}
