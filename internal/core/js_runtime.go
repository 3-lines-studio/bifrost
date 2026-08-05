package core

import "strings"

const (
	JSRuntimeSobek = "sobek"
	JSRuntimeBun   = "bun"
)

// NormalizeJSRuntime returns the selected JavaScript backend. Sobek is the
// default; Bun must be selected explicitly.
func NormalizeJSRuntime(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), JSRuntimeBun) {
		return JSRuntimeBun
	}
	return JSRuntimeSobek
}
