//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/3-lines-studio/bifrost/mygo/upload"
)

// serverURL is empty in a browser: a request names the page's own origin, so
// the build works from any host that serves it.
func serverURL() string {
	return ""
}

// publicURL is where the page is, which a link the app shares starts with.
func publicURL() string {
	return js.Global().Get("location").Get("origin").String()
}

// uploadStore keeps the parts a server has across a reload.
func uploadStore() upload.Store {
	return upload.Browser()
}
