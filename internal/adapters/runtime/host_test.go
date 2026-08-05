package runtime

import (
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestHostUsesManifestRuntimeWhenEnvironmentIsUnset(t *testing.T) {
	t.Setenv("BIFROST_JS_RUNTIME", "")
	h := &Host{manifest: &core.Manifest{Runtime: "sobek"}}
	if !h.useSobek() {
		t.Fatal("expected Sobek runtime from manifest")
	}

	t.Setenv("BIFROST_JS_RUNTIME", "bun")
	if h.useSobek() {
		t.Fatal("explicit environment must override the manifest")
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
