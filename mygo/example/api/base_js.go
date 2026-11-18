//go:build js && wasm

package api

import "syscall/js"

// DefaultBaseURL is the page's origin: in a browser the server is the one
// that served the app.
func DefaultBaseURL() string {
	location := js.Global().Get("location")
	if location.IsUndefined() || location.IsNull() {
		return ""
	}
	return location.Get("origin").String()
}
