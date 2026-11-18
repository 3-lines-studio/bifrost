//go:build !js

package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/3-lines-studio/bifrost/mygo/upload"
	"github.com/egoist/mygo"
)

// serverURL is the server the desktop app is a client of.
func serverURL() string {
	if value := os.Getenv("BIFROST_DEMO_SERVER"); value != "" {
		return strings.TrimSuffix(value, "/")
	}
	return "http://localhost:8080"
}

// publicURL is where the server is, which a link the app shares starts with.
func publicURL() string { return serverURL() }

// uploadStore keeps the parts a server has on the disk of the app, so a resume
// survives it closing.
func uploadStore() upload.Store {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return upload.NewMemory()
	}
	return upload.Disk(filepath.Join(dir, "uploads"))
}
