package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateGoArity(t *testing.T) {
	dir := t.TempDir()
	source := `package page
type Props struct{}
func Load(value string, extra string) (Props, int) { return Props{}, 0 }
func Page() {}
`
	if err := os.WriteFile(filepath.Join(dir, "page.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	err := validateGoArity(dir, goArity{name: "Load", params: 1, results: 2}, goArity{name: "Page", params: 1, results: 1})
	if err == nil || !strings.Contains(err.Error(), "Load must have 1 parameters and 2 results, got 2 and 2") || !strings.Contains(err.Error(), "Page must have 1 parameters and 1 results, got 0 and 0") {
		t.Fatalf("error = %v", err)
	}
}
