package usecase

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePublicDir_PrefersAppRoot(t *testing.T) {
	tmpDir := t.TempDir()
	appRoot := filepath.Join(tmpDir, "cmd", "server")
	writeTestFile(t, filepath.Join(appRoot, "public", "favicon.ico"), "icon")
	writeTestFile(t, filepath.Join(tmpDir, "public", "favicon.ico"), "module-icon")

	got := resolvePublicDir(appRoot, tmpDir)
	want := filepath.Join(appRoot, "public")
	if got != want {
		t.Fatalf("resolvePublicDir() = %s, want %s", got, want)
	}
}

func TestResolvePublicDir_FallsBackToModuleRoot(t *testing.T) {
	tmpDir := t.TempDir()
	appRoot := filepath.Join(tmpDir, "cmd", "server")
	writeTestFile(t, filepath.Join(tmpDir, "public", "favicon.ico"), "icon")

	got := resolvePublicDir(appRoot, tmpDir)
	want := filepath.Join(tmpDir, "public")
	if got != want {
		t.Fatalf("resolvePublicDir() = %s, want %s", got, want)
	}
}

func TestResolvePublicDir_IgnoresFileAtAppRoot(t *testing.T) {
	tmpDir := t.TempDir()
	appRoot := filepath.Join(tmpDir, "cmd", "server")
	writeTestFile(t, filepath.Join(appRoot, "public"), "not a dir")
	writeTestFile(t, filepath.Join(tmpDir, "public", "favicon.ico"), "icon")

	got := resolvePublicDir(appRoot, tmpDir)
	want := filepath.Join(tmpDir, "public")
	if got != want {
		t.Fatalf("resolvePublicDir() = %s, want %s", got, want)
	}
}

func TestResolvePublicDir_MissingPublicReturnsModuleRoot(t *testing.T) {
	tmpDir := t.TempDir()
	appRoot := filepath.Join(tmpDir, "cmd", "server")
	if err := os.MkdirAll(appRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	got := resolvePublicDir(appRoot, tmpDir)
	want := filepath.Join(tmpDir, "public")
	if got != want {
		t.Fatalf("resolvePublicDir() = %s, want %s", got, want)
	}
}
