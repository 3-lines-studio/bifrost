package app

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestShouldPrintRouteTable_DefaultInTests(t *testing.T) {
	t.Setenv("BIFROST_ROUTE_TABLE", "")
	t.Setenv("BIFROST_NO_ROUTE_TABLE", "")

	// In `go test`, stdout is a pipe, not a terminal, so the default is false.
	if shouldPrintRouteTable() {
		t.Error("shouldPrintRouteTable() = true in tests, want false (stdout is not a terminal)")
	}
}

func TestShouldPrintRouteTable_ForceWithEnv(t *testing.T) {
	t.Setenv("BIFROST_ROUTE_TABLE", "1")
	t.Setenv("BIFROST_NO_ROUTE_TABLE", "")

	if !shouldPrintRouteTable() {
		t.Error("BIFROST_ROUTE_TABLE=1 should force route table printing")
	}
}

func TestShouldPrintRouteTable_SuppressWithEnv(t *testing.T) {
	t.Setenv("BIFROST_ROUTE_TABLE", "1")
	t.Setenv("BIFROST_NO_ROUTE_TABLE", "1")

	if shouldPrintRouteTable() {
		t.Error("BIFROST_NO_ROUTE_TABLE=1 should suppress route table even when BIFROST_ROUTE_TABLE=1")
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close write end of stdout pipe: %v", err)
	}
	os.Stdout = oldStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed to read captured stdout: %v", err)
	}
	return buf.String()
}

func TestPrintRouteTable_FormatsRoutes(t *testing.T) {
	routes := []core.Route{
		core.Page("/", "./pages/home.tsx"),
		core.Page("/about", "./pages/about.tsx", core.WithClient()),
		core.Page("/product", "./pages/product.tsx", core.WithStatic()),
	}

	out := captureStdout(t, func() {
		printRouteTable(routes)
	})

	if !strings.Contains(out, "Bifrost routes:") {
		t.Errorf("expected header 'Bifrost routes:' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "PATTERN") {
		t.Errorf("expected 'PATTERN' column header, got:\n%s", out)
	}
	if !strings.Contains(out, "COMPONENT") {
		t.Errorf("expected 'COMPONENT' column header, got:\n%s", out)
	}
	if !strings.Contains(out, "MODE") {
		t.Errorf("expected 'MODE' column header, got:\n%s", out)
	}
	if !strings.Contains(out, "/") {
		t.Errorf("expected root route '/', got:\n%s", out)
	}
	if !strings.Contains(out, "./pages/home.tsx") {
		t.Errorf("expected home component path, got:\n%s", out)
	}
	if !strings.Contains(out, "client") {
		t.Errorf("expected client mode label, got:\n%s", out)
	}
	if !strings.Contains(out, "static") {
		t.Errorf("expected static mode label, got:\n%s", out)
	}
}

func TestPrintRouteTable_EmptyRoutes(t *testing.T) {
	out := captureStdout(t, func() {
		printRouteTable(nil)
	})

	if out != "" {
		t.Errorf("expected no output for empty routes, got:\n%s", out)
	}
}

func TestPrintRouteTable_ModeLabels(t *testing.T) {
	routes := []core.Route{
		core.Page("/ssr", "./ssr.tsx"),
		core.Page("/client", "./client.tsx", core.WithClient()),
		core.Page("/static", "./static.tsx", core.WithStatic()),
	}

	out := captureStdout(t, func() {
		printRouteTable(routes)
	})

	for _, want := range []string{"ssr", "client", "static"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain mode %q, got:\n%s", want, out)
		}
	}
}
