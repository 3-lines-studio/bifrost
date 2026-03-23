package usecase

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAbsoluteComponentPath(t *testing.T) {
	t.Parallel()
	cwd := "/proj/root"
	if got := AbsoluteComponentPath(cwd, "./pages/home.tsx"); got != filepath.Join(cwd, "pages", "home.tsx") {
		t.Fatalf("got %q", got)
	}
	if got := AbsoluteComponentPath(cwd, "pages/about.tsx"); got != filepath.Join(cwd, "pages", "about.tsx") {
		t.Fatalf("got %q", got)
	}
}

func TestCalculateImportPath(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	entriesDir := filepath.Join(base, ".bifrost", "entries")
	if err := os.MkdirAll(entriesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pagesDir := filepath.Join(base, "pages")
	if err := os.MkdirAll(pagesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	comp := filepath.Join(pagesDir, "home.tsx")
	if err := os.WriteFile(comp, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(entriesDir, "home.tsx")
	rel, err := CalculateImportPath(entry, comp)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.Abs(filepath.Join(filepath.Dir(entry), rel))
	if err != nil {
		t.Fatal(err)
	}
	wantAbs, err := filepath.Abs(comp)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != wantAbs {
		t.Fatalf("resolved %q want %q (rel was %q)", resolved, wantAbs, rel)
	}
}
