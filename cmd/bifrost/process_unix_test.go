//go:build unix

package main

import (
	"os/exec"
	"testing"
)

func TestSignalProcessTree_KillsRunningChild(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	setSysProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start sleep: %v", err)
	}

	signalProcessTree(cmd)

	err := cmd.Wait()
	if err == nil {
		t.Fatal("expected process to have been terminated, not exit successfully")
	}
}

func TestSignalProcessTree_NilCmd(t *testing.T) {
	signalProcessTree(nil)
	forceKillProcessTree(nil)
}

func TestSignalProcessTree_AlreadyExited(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start true: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("failed to wait for true: %v", err)
	}

	signalProcessTree(cmd)
	forceKillProcessTree(cmd)
}
