package runtime

import (
	"embed"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestHostDefaultsToSobek(t *testing.T) {
	t.Setenv("BIFROST_JS_RUNTIME", "")
	if !(&Host{}).useSobek() {
		t.Fatal("expected Sobek as the default runtime")
	}
}

func TestHostUsesManifestRuntimeWhenEnvironmentIsUnset(t *testing.T) {
	t.Setenv("BIFROST_JS_RUNTIME", "")
	h := &Host{manifest: &core.Manifest{Runtime: "bun"}}
	if h.useSobek() {
		t.Fatal("expected Bun runtime from manifest")
	}

	t.Setenv("BIFROST_JS_RUNTIME", "sobek")
	if !h.useSobek() {
		t.Fatal("explicit environment must override the manifest")
	}
}

func TestExportHostRecognizesLegacyBunManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"entries":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	runtimePath := filepath.Join(root, "runtime", "bifrost-renderer")
	if runtime.GOOS == "windows" {
		runtimePath += ".exe"
	}
	if err := os.MkdirAll(filepath.Dir(runtimePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtimePath, []byte("legacy"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BIFROST_EXPORT_DIR", root)
	t.Setenv("BIFROST_JS_RUNTIME", "")
	h, err := NewHost(embed.FS{}, core.ModeExport)
	if err != nil {
		t.Fatal(err)
	}
	if h.Manifest().Runtime != core.JSRuntimeBun {
		t.Fatalf("legacy manifest runtime = %q, want bun", h.Manifest().Runtime)
	}
}

func TestHostStopRunsCleanupOnce(t *testing.T) {
	calls := 0
	h := &Host{ssrCleanup: func() { calls++ }}

	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", calls)
	}
}
