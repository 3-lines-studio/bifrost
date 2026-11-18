//go:build !js

package api

// DefaultBaseURL is empty on native builds: the app names its server.
func DefaultBaseURL() string { return "" }
