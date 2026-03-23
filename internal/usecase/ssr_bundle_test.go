package usecase

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/adapters/framework"
	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestNormalizeSSRBundleLeavesFlatOutputInPlace(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	expected := filepath.Join(dir, "pages-home-entry-ssr.js")
	writeTestFile(t, expected, "// ssr")

	got, err := normalizeSSRBundle(dir, "pages-home-entry")
	if err != nil {
		t.Fatalf("normalizeSSRBundle() error = %v", err)
	}
	if got != expected {
		t.Fatalf("normalizeSSRBundle() path = %q, want %q", got, expected)
	}
}

func TestNormalizeSSRBundleMovesNestedOutput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	nested := filepath.Join(dir, ".bifrost", "entries", "pages-home-entry-ssr.js")
	expected := filepath.Join(dir, "pages-home-entry-ssr.js")
	writeTestFile(t, nested, "// ssr")
	writeTestFile(t, filepath.Join(dir, ".bifrost", "entries", "pages-home-entry-ssr.css"), "/* css */")

	got, err := normalizeSSRBundle(dir, "pages-home-entry")
	if err != nil {
		t.Fatalf("normalizeSSRBundle() error = %v", err)
	}
	if got != expected {
		t.Fatalf("normalizeSSRBundle() path = %q, want %q", got, expected)
	}
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected moved SSR bundle: %v", err)
	}
	if _, err := os.Stat(nested); !os.IsNotExist(err) {
		t.Fatalf("expected nested SSR bundle removed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".bifrost", "entries", "pages-home-entry-ssr.css")); err != nil {
		t.Fatalf("expected SSR css to remain untouched: %v", err)
	}
}

func TestNormalizeSSRBundleErrorsWhenMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, err := normalizeSSRBundle(dir, "pages-home-entry")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "expected SSR bundle at") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeSSRBundleErrorsWhenMultipleNestedOutputsExist(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ".bifrost", "entries", "pages-home-entry-ssr.js"), "// one")
	writeTestFile(t, filepath.Join(dir, "nested", "pages-home-entry-ssr.js"), "// two")

	_, err := normalizeSSRBundle(dir, "pages-home-entry")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "multiple nested SSR bundles found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompileDevPageOnDemandNormalizesNestedSSRBundle(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "pages", "home.tsx"), "export default function Page(){ return <div>Hello</div> }")

	renderer := &fakeRenderer{
		buildFn: func(entrypoints []string, outdir string, entryNames []string) (map[string]core.ClientBuildResult, error) {
			return map[string]core.ClientBuildResult{
				entryNames[0]: {Script: "/dist/" + entryNames[0] + ".js"},
			}, nil
		},
		buildSSRFn: func(entrypoints []string, outdir string) error {
			name := strings.TrimSuffix(filepath.Base(entrypoints[0]), filepath.Ext(entrypoints[0]))
			writeTestFile(t, filepath.Join(outdir, ".bifrost", "entries", name+".js"), "// ssr")
			return nil
		},
	}

	err := CompileDevPageOnDemand(
		renderer,
		tmpDir,
		"pages-home-entry",
		core.PageConfig{ComponentPath: "./pages/home.tsx", Mode: core.ModeSSR},
		framework.DefaultAdapter(),
	)
	if err != nil {
		t.Fatalf("CompileDevPageOnDemand() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".bifrost", "ssr", "pages-home-entry-ssr.js")); err != nil {
		t.Fatalf("expected normalized SSR bundle: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".bifrost", "ssr", ".bifrost", "entries", "pages-home-entry-ssr.js")); !os.IsNotExist(err) {
		t.Fatalf("expected nested SSR bundle removed, got %v", err)
	}
}
