package runtime

import "testing"

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
