package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close stdout pipe: %v", err)
	}
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed to read captured stdout: %v", err)
	}
	return buf.String()
}

func TestPrintBuildUsage_DocumentedOrdering(t *testing.T) {
	out := captureStdout(t, printBuildUsage)

	if !strings.Contains(out, "Usage: bifrost build <main.go> [flags]") {
		t.Errorf("usage does not show documented ordering; got:\n%s", out)
	}
}

func TestResolveSobekPGO(t *testing.T) {
	t.Run("default embedded profile", func(t *testing.T) {
		t.Setenv("BIFROST_SOBEK_PGO", "")
		path, cleanup, err := resolveSobekPGO(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() == 0 {
			t.Fatal("embedded PGO profile is empty")
		}
	})

	t.Run("disabled", func(t *testing.T) {
		t.Setenv("BIFROST_SOBEK_PGO", "off")
		path, cleanup, err := resolveSobekPGO(t.TempDir())
		defer cleanup()
		if err != nil {
			t.Fatal(err)
		}
		if path != "" {
			t.Fatalf("path = %q, want empty", path)
		}
	})

	t.Run("configured profile", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "custom.pgo")
		if err := os.WriteFile(path, []byte("profile"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("BIFROST_SOBEK_PGO", "custom.pgo")
		got, cleanup, err := resolveSobekPGO(root)
		defer cleanup()
		if err != nil {
			t.Fatal(err)
		}
		if got != path {
			t.Fatalf("path = %q, want %q", got, path)
		}
	})
}

func TestParseFlags_GoBuildDefault(t *testing.T) {
	mf, gb, _ := parseFlags([]string{"./main.go", "--go-build"})
	if mf != "./main.go" {
		t.Errorf("expected mainFile './main.go', got '%s'", mf)
	}
	if gb != "./tmp/app" {
		t.Errorf("expected goBuildOutput './tmp/app', got '%s'", gb)
	}
}

func TestParseFlags_GoBuildEquals(t *testing.T) {
	mf, gb, _ := parseFlags([]string{"./main.go", "--go-build=./myapp"})
	if mf != "./main.go" {
		t.Errorf("expected mainFile './main.go', got '%s'", mf)
	}
	if gb != "./myapp" {
		t.Errorf("expected goBuildOutput './myapp', got '%s'", gb)
	}
}

func TestParseFlags_GoBuildSpace(t *testing.T) {
	mf, gb, _ := parseFlags([]string{"./main.go", "--go-build", "./myapp"})
	if mf != "./main.go" {
		t.Errorf("expected mainFile './main.go', got '%s'", mf)
	}
	if gb != "./myapp" {
		t.Errorf("expected goBuildOutput './myapp', got '%s'", gb)
	}
}

func TestParseFlags_NoGoBuild(t *testing.T) {
	mf, gb, _ := parseFlags([]string{"./main.go"})
	if mf != "./main.go" {
		t.Errorf("expected mainFile './main.go', got '%s'", mf)
	}
	if gb != "" {
		t.Errorf("expected goBuildOutput '', got '%s'", gb)
	}
}

func TestParseFlags_GoBuildFollowedByFlag(t *testing.T) {
	mf, gb, _ := parseFlags([]string{"./main.go", "--go-build", "--other"})
	if mf != "./main.go" {
		t.Errorf("expected mainFile './main.go', got '%s'", mf)
	}
	if gb != "./tmp/app" {
		t.Errorf("expected goBuildOutput './tmp/app' (fallback), got '%s'", gb)
	}
}

// The documented CLI contract requires the main file to precede --go-build.
// If the flag is placed first, parseFlags currently interprets the main file
// as the --go-build output path and leaves mainFile empty.
func TestParseFlags_GoBuildBeforeMainFile(t *testing.T) {
	mf, gb, _ := parseFlags([]string{"--go-build", "./main.go"})
	if mf != "" {
		t.Errorf("expected mainFile to be empty when --go-build precedes it, got '%s'", mf)
	}
	if gb != "./main.go" {
		t.Errorf("expected goBuildOutput './main.go', got '%s'", gb)
	}
}
