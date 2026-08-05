package usecase

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyPublicDirRejectsSymlinks(t *testing.T) {
	src := filepath.Join(t.TempDir(), "public")
	dst := filepath.Join(t.TempDir(), "output")
	writeTestFile(t, filepath.Join(src, "asset.txt"), "asset")
	outside := filepath.Join(t.TempDir(), "secret.txt")
	writeTestFile(t, outside, "secret")
	if err := os.Symlink(outside, filepath.Join(src, "linked-secret.txt")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	svc := &BuildService{}
	if err := svc.copyPublicDir(src, dst); err == nil {
		t.Fatal("expected public symlink to return an error")
	}
	if _, err := os.Stat(filepath.Join(dst, "linked-secret.txt")); !os.IsNotExist(err) {
		t.Fatalf("linked file was copied: %v", err)
	}
}

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
