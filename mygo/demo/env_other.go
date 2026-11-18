//go:build !js

package main

import (
	"os"
	"strings"
)

// serverURL is the server the desktop app is a client of.
func serverURL() string {
	if value := os.Getenv("BIFROST_DEMO_SERVER"); value != "" {
		return strings.TrimSuffix(value, "/")
	}
	return "http://localhost:8080"
}
