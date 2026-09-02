//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestConventionReleaseGate(t *testing.T) {
	if os.Getenv("BIFROST_RELEASE_GATE") == "" {
		t.Skip("set BIFROST_RELEASE_GATE=1")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(filepath.Join(root, "scripts", "convention-release-gate.sh"))
	command.Dir = root
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
}
