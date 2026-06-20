package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestParseDevFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantMain    string
		wantPort    int
		wantAppPort int
	}{
		{
			name:        "defaults",
			args:        []string{"./main.go"},
			wantMain:    "./main.go",
			wantPort:    3000,
			wantAppPort: 8080,
		},
		{
			name:        "port space",
			args:        []string{"./main.go", "--port", "4000"},
			wantMain:    "./main.go",
			wantPort:    4000,
			wantAppPort: 8080,
		},
		{
			name:        "port equals before file",
			args:        []string{"--port=4000", "./main.go"},
			wantMain:    "./main.go",
			wantPort:    4000,
			wantAppPort: 8080,
		},
		{
			name:        "app port",
			args:        []string{"./main.go", "--app-port", "9090"},
			wantMain:    "./main.go",
			wantPort:    3000,
			wantAppPort: 9090,
		},
		{
			name:        "all flags",
			args:        []string{"--port=5000", "--app-port=9000", "./main.go"},
			wantMain:    "./main.go",
			wantPort:    5000,
			wantAppPort: 9000,
		},
		{
			name:        "invalid port ignored",
			args:        []string{"./main.go", "--port", "not-a-number"},
			wantMain:    "./main.go",
			wantPort:    3000,
			wantAppPort: 8080,
		},
		{
			name:        "port missing value at end",
			args:        []string{"./main.go", "--port"},
			wantMain:    "./main.go",
			wantPort:    3000,
			wantAppPort: 8080,
		},
		{
			name:        "extra args ignored",
			args:        []string{"./main.go", "--port", "4000", "extra"},
			wantMain:    "./main.go",
			wantPort:    4000,
			wantAppPort: 8080,
		},
		{
			name:        "no main file",
			args:        []string{},
			wantMain:    "",
			wantPort:    3000,
			wantAppPort: 8080,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mainFile, port, appPort := parseDevFlags(tt.args)
			if mainFile != tt.wantMain {
				t.Errorf("mainFile = %q, want %q", mainFile, tt.wantMain)
			}
			if port != tt.wantPort {
				t.Errorf("port = %d, want %d", port, tt.wantPort)
			}
			if appPort != tt.wantAppPort {
				t.Errorf("appPort = %d, want %d", appPort, tt.wantAppPort)
			}
		})
	}
}

func TestManagedChild_StartFailure(t *testing.T) {
	mc := startManagedChild(t.TempDir(), "/nonexistent/binary/path")
	if mc != nil {
		t.Fatal("expected nil managedChild for missing binary")
	}
}

func TestManagedChild_ReportsExitCode(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("package main\nimport \"os\"\nfunc main() { os.Exit(42) }\n"), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	bin := filepath.Join(dir, "bin")
	if out, err := exec.Command("go", "build", "-o", bin, src).CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v\n%s", err, out)
	}

	mc := startManagedChild(dir, bin)
	if mc == nil {
		t.Fatal("expected managedChild")
	}

	select {
	case code := <-mc.exitCh:
		if code != 42 {
			t.Errorf("exit code = %d, want 42", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for child exit")
	}
}

func TestManagedChild_StopTerminatesRunningChild(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("package main\nimport \"time\"\nfunc main() { time.Sleep(30 * time.Second) }\n"), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	bin := filepath.Join(dir, "bin")
	if out, err := exec.Command("go", "build", "-o", bin, src).CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v\n%s", err, out)
	}

	mc := startManagedChild(dir, bin)
	if mc == nil {
		t.Fatal("expected managedChild")
	}

	done := make(chan struct{})
	go func() {
		mc.stop()
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not complete in time")
	}

	// Ensure the underlying process is gone.
	if mc.cmd != nil && mc.cmd.Process != nil {
		if err := mc.cmd.Process.Kill(); err == nil {
			t.Error("process was still running after stop")
		}
	}
}
