//go:build js && wasm

package main

// serverURL is empty in a browser: a request names the page's own origin, so
// the build works from any host that serves it.
func serverURL() string {
	return ""
}
