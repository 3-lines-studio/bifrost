package process

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func skipIfNoBunForPlugin(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun not available, skipping plugin integration test")
	}
}

func svelteTSFixture(t *testing.T) (exampleDir, fixture string) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get caller path")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	exampleDir = filepath.Join(root, "example", "cmd", "full")
	fixture = filepath.Join(exampleDir, "pages", "svelte", "avatar-context.svelte.ts")
	return exampleDir, fixture
}

func withExampleCWD(t *testing.T, exampleDir string) func() {
	t.Helper()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	if err := os.Chdir(exampleDir); err != nil {
		t.Fatalf("failed to chdir to example dir: %v", err)
	}
	return func() { _ = os.Chdir(origDir) }
}

func assertNoTypeScriptArtifacts(t *testing.T, code, label string) {
	t.Helper()
	if strings.Contains(code, "export type") || strings.Contains(code, "export interface") {
		t.Errorf("%s still contains TypeScript type declarations", label)
	}
}

func assertHasRuntimeExports(t *testing.T, code, label string) {
	t.Helper()
	if !strings.Contains(code, "createModuleContext") || !strings.Contains(code, "getModuleContext") {
		t.Errorf("%s is missing expected runtime exports", label)
	}
}

func TestSveltePlugin_CompilesSvelteTSModule_SSR(t *testing.T) {
	skipIfNoBunForPlugin(t)

	exampleDir, fixture := svelteTSFixture(t)
	cleanup := withExampleCWD(t, exampleDir)
	defer cleanup()

	outDir := t.TempDir()

	r, err := NewRenderer(core.ModeProd, RuntimeSource(core.ModeProd, core.FrameworkSvelte))
	if err != nil {
		t.Fatalf("failed to start renderer: %v", err)
	}
	defer func() {
		if err := r.Stop(); err != nil {
			t.Errorf("renderer stop failed: %v", err)
		}
	}()

	if err := r.BuildSSR([]string{fixture}, outDir, "svelte"); err != nil {
		t.Fatalf("BuildSSR failed for .svelte.ts module: %v", err)
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("failed to read outdir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one SSR build output file")
	}

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".js") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(outDir, e.Name()))
		if err != nil {
			t.Fatalf("failed to read output %s: %v", e.Name(), err)
		}
		code := string(content)
		assertNoTypeScriptArtifacts(t, code, e.Name())
		assertHasRuntimeExports(t, code, e.Name())
		break
	}
}

func TestSveltePlugin_CompilesSvelteTSModule_Client(t *testing.T) {
	skipIfNoBunForPlugin(t)

	exampleDir, fixture := svelteTSFixture(t)
	cleanup := withExampleCWD(t, exampleDir)
	defer cleanup()

	outDir := t.TempDir()

	r, err := NewRenderer(core.ModeProd, RuntimeSource(core.ModeProd, core.FrameworkSvelte))
	if err != nil {
		t.Fatalf("failed to start renderer: %v", err)
	}
	defer func() {
		if err := r.Stop(); err != nil {
			t.Errorf("renderer stop failed: %v", err)
		}
	}()

	if _, err := r.Build([]string{fixture}, outDir, []string{"ts-module"}, "svelte"); err != nil {
		t.Fatalf("Build failed for .svelte.ts module: %v", err)
	}

	outPath := filepath.Join(outDir, "ts-module.js")
	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read client output: %v", err)
	}
	code := string(content)
	assertNoTypeScriptArtifacts(t, code, "client output")
	assertHasRuntimeExports(t, code, "client output")
}
