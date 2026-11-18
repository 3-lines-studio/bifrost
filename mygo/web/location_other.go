//go:build !js

package web

// FragmentLocation does nothing outside a browser: a window has no URL.
func FragmentLocation() {}
