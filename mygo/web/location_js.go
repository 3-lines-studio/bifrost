//go:build js && wasm

package web

import "syscall/js"

// FragmentLocation puts the path of the page in the URL's fragment, where
// MyGo's router follows the tab's history: an app that routes with ui.Router
// calls it before it builds its first frame, so /pricing becomes #/pricing and
// the router shows the page the server served.
//
// An app that reads location.pathname itself, as one whose pages the server
// answers one by one, leaves the URL as it is and does not call it.
func FragmentLocation() {
	location := js.Global().Get("location")
	path := location.Get("pathname").String()
	if path == "/" || location.Get("hash").String() != "" {
		return
	}
	js.Global().Get("history").Call("replaceState", js.Null(), "", "#"+path)
}
