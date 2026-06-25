package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var cliBinPath string

func TestMain(m *testing.M) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("failed to get test file path")
	}
	pkgPath, err := filepath.Abs(filepath.Dir(file))
	if err != nil {
		panic(err)
	}
	repoRoot := filepath.Dir(filepath.Dir(pkgPath))

	bin := filepath.Join(os.TempDir(), "bifrost-test-cli")
	cmd := exec.Command("go", "build", "-o", bin, pkgPath)
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		panic(string(out))
	}
	cliBinPath = bin

	code := m.Run()
	_ = os.Remove(bin)
	os.Exit(code)
}

func runCLI(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(cliBinPath, args...)
	cmd.Dir = t.TempDir()
	return cmd
}

func TestCLI_NoArgsPrintsUsage(t *testing.T) {
	cmd := runCLI(t)
	out, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatal("expected non-zero exit for no arguments")
	}
	if !strings.Contains(string(out), "Bifrost CLI") {
		t.Errorf("expected usage header, got:\n%s", out)
	}
	if !strings.Contains(string(out), "Usage: bifrost <subcommand>") {
		t.Errorf("expected usage line, got:\n%s", out)
	}
}

func TestCLI_RootHelp(t *testing.T) {
	for _, flag := range []string{"--help", "-h", "help"} {
		t.Run(flag, func(t *testing.T) {
			cmd := runCLI(t, flag)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("expected zero exit for %s, got error: %v\n%s", flag, err, out)
			}
			if !strings.Contains(string(out), "Subcommands:") {
				t.Errorf("expected subcommands list, got:\n%s", out)
			}
		})
	}
}

func TestCLI_UnknownSubcommand(t *testing.T) {
	cmd := runCLI(t, "unknown")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for unknown subcommand")
	}
	if !strings.Contains(string(out), "Unknown subcommand: unknown") {
		t.Errorf("expected unknown subcommand error, got:\n%s", out)
	}
}

func TestCLI_InitHelp(t *testing.T) {
	cmd := runCLI(t, "init", "--help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected zero exit for init --help, got error: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "bifrost init") {
		t.Errorf("expected init usage, got:\n%s", out)
	}
}

func TestCLI_InitMissingDir(t *testing.T) {
	cmd := runCLI(t, "init")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for init without project dir")
	}
	if !strings.Contains(string(out), "bifrost init") {
		t.Errorf("expected init usage, got:\n%s", out)
	}
}

func TestCLI_DoctorHelp(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		t.Run(flag, func(t *testing.T) {
			cmd := runCLI(t, "doctor", flag)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("expected zero exit for doctor %s, got error: %v\n%s", flag, err, out)
			}
			if !strings.Contains(string(out), "bifrost doctor") {
				t.Errorf("expected doctor usage, got:\n%s", out)
			}
		})
	}
}

func TestCLI_DoctorRepairsDirectory(t *testing.T) {
	dir := t.TempDir()
	cmd := runCLI(t, "doctor", dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Repair complete!") {
		t.Errorf("expected repair completion message, got:\n%s", out)
	}
}

func TestCLI_BuildHelp(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		t.Run(flag, func(t *testing.T) {
			cmd := runCLI(t, "build", flag)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("expected zero exit for build %s, got error: %v\n%s", flag, err, out)
			}
			if !strings.Contains(string(out), "bifrost build") {
				t.Errorf("expected build usage, got:\n%s", out)
			}
			if !strings.Contains(string(out), "(react)") {
				t.Errorf("expected framework list to include (react), got:\n%s", out)
			}
			if !strings.Contains(string(out), "--go-build") {
				t.Errorf("expected --go-build flag in build usage, got:\n%s", out)
			}
		})
	}
}

func TestCLI_DevHelp(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		t.Run(flag, func(t *testing.T) {
			cmd := runCLI(t, "dev", flag)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("expected zero exit for dev %s, got error: %v\n%s", flag, err, out)
			}
			if !strings.Contains(string(out), "bifrost dev") {
				t.Errorf("expected dev usage, got:\n%s", out)
			}
			if !strings.Contains(string(out), "--port") {
				t.Errorf("expected --port flag in dev usage, got:\n%s", out)
			}
		})
	}
}

func TestCLI_DevMissingFile(t *testing.T) {
	cmd := runCLI(t, "dev")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for dev without main file")
	}
	if !strings.Contains(string(out), "Missing main.go file argument") {
		t.Errorf("expected missing file error, got:\n%s", out)
	}
}

func TestCLI_BuildMissingFile(t *testing.T) {
	cmd := runCLI(t, "build")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for build without main file")
	}
	if !strings.Contains(string(out), "Missing main.go file argument") {
		t.Errorf("expected missing file error, got:\n%s", out)
	}
}

func TestCLI_InitScaffoldsMinimalProject(t *testing.T) {
	dir := t.TempDir()
	cmd := runCLI(t, "init", dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}

	expected := []string{
		filepath.Join(dir, "main.go"),
		filepath.Join(dir, "pages", "home.tsx"),
		filepath.Join(dir, "package.json"),
	}
	for _, path := range expected {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected scaffolded file %s to exist: %v", path, err)
		}
	}
}
