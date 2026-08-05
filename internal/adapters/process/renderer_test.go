package process

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestFormatRenderError_CompileError(t *testing.T) {
	e := &bunErrorJSON{
		Message: "Build failed - Expected whitespace https://react.dev/errors/expected_whitespace (expected_whitespace)",
	}

	se := formatRenderError(e)
	if se == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := any(se).(*core.StructuredError); !ok {
		t.Fatalf("expected *core.StructuredError, got %T", se)
	}

	errStr := se.Error()
	if !strings.Contains(errStr, "expected_whitespace") {
		t.Errorf("expected 'expected_whitespace' in error, got: %s", errStr)
	}
	if !strings.Contains(errStr, "react.dev/errors/expected_whitespace") {
		t.Errorf("expected react.dev URL in error, got: %s", errStr)
	}
}

func TestFormatRenderError_WithStack(t *testing.T) {
	e := &bunErrorJSON{
		Message: "Build failed",
		Stack:   "at compile (/src/compiler.js:123:4)",
	}

	se := formatRenderError(e)
	if se == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := any(se).(*core.StructuredError); !ok {
		t.Fatalf("expected *core.StructuredError, got %T", se)
	}

	errStr := se.Error()
	if !strings.Contains(errStr, "Build failed") {
		t.Errorf("expected 'Build failed' in error, got: %s", errStr)
	}
	if !strings.Contains(errStr, "at compile") {
		t.Errorf("expected stack trace in error, got: %s", errStr)
	}
}

func TestFormatRenderError_WithSubErrors(t *testing.T) {
	e := &bunErrorJSON{
		Message: "Build failed",
		Errors: []errorDetailJSON{
			{Message: "Expected whitespace", Stack: "line 2"},
			{Message: "Unexpected token", Stack: "line 3"},
		},
	}

	se := formatRenderError(e)
	if se == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := any(se).(*core.StructuredError); !ok {
		t.Fatalf("expected *core.StructuredError, got %T", se)
	}

	errStr := se.Error()
	if !strings.Contains(errStr, "Expected whitespace") {
		t.Errorf("expected 'Expected whitespace' in error, got: %s", errStr)
	}
	if !strings.Contains(errStr, "Unexpected token") {
		t.Errorf("expected 'Unexpected token' in error, got: %s", errStr)
	}
	if !strings.Contains(errStr, "Errors:") {
		t.Errorf("expected 'Errors:' section header, got: %s", errStr)
	}
}

func TestFormatRenderError_Nil(t *testing.T) {
	se := formatRenderError(nil)
	if se != nil {
		t.Fatalf("expected nil for nil input, got: %v", se)
	}
}

func TestFormatRenderError_WithPosition(t *testing.T) {
	e := &bunErrorJSON{
		Message: "Build failed",
		Errors: []errorDetailJSON{
			{
				Message: "Expected whitespace",
				Position: &errorPositionJSON{
					File:     "src/App.tsx",
					Line:     2,
					Column:   1,
					LineText: "{#if}",
				},
			},
		},
	}

	se := formatRenderError(e)
	if se == nil {
		t.Fatal("expected error, got nil")
	}

	if len(se.SubErrors) != 1 {
		t.Fatalf("expected 1 sub-error, got %d", len(se.SubErrors))
	}
	sub := se.SubErrors[0]
	if sub.File != "src/App.tsx" {
		t.Errorf("expected file 'src/App.tsx', got %q", sub.File)
	}
	if sub.Line != 2 {
		t.Errorf("expected line 2, got %d", sub.Line)
	}
	if sub.Column != 1 {
		t.Errorf("expected column 1, got %d", sub.Column)
	}
	if sub.LineText != "{#if}" {
		t.Errorf("expected LineText '{#if}', got %q", sub.LineText)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRendererRenderContextHonorsCancellation(t *testing.T) {
	r := &Renderer{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.RenderContext(ctx, "/page.js", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want %v", err, context.Canceled)
	}
}

func TestRendererSocketUsesPrivateTempDirectory(t *testing.T) {
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun is not available")
	}

	r, err := NewRenderer(core.ModeDev, `Bun.serve({ unix: process.env.BIFROST_SOCKET, fetch() { return new Response("ok") } });`)
	if err != nil {
		t.Fatal(err)
	}
	socketDir := filepath.Dir(r.socket)

	info, err := os.Stat(socketDir)
	if err != nil {
		_ = r.Stop()
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		_ = r.Stop()
		t.Fatalf("socket directory mode = %o, want 700", got)
	}
	if err := r.Stop(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(socketDir); !os.IsNotExist(err) {
		t.Fatalf("socket directory still exists after Stop: %v", err)
	}
}

func TestRendererExitsAndCleansSocketAfterParentExit(t *testing.T) {
	if os.Getenv("BIFROST_PARENT_EXIT_HELPER") == "1" {
		renderer, err := NewRenderer(core.ModeProd, RuntimeSource(core.ModeProd), "BIFROST_DEV=0")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Printf("BIFROST_HELPER %d %s\n", renderer.cmd.Process.Pid, filepath.Dir(renderer.socket))
		os.Exit(0)
	}
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun is not available")
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestRendererExitsAndCleansSocketAfterParentExit$")
	cmd.Env = append(os.Environ(), "BIFROST_PARENT_EXIT_HELPER=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper failed: %v\n%s", err, output)
	}
	var childPID int
	var socketDir string
	for line := range strings.SplitSeq(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[0] != "BIFROST_HELPER" {
			continue
		}
		childPID, err = strconv.Atoi(fields[1])
		if err != nil {
			t.Fatal(err)
		}
		socketDir = fields[2]
	}
	if childPID == 0 || socketDir == "" {
		t.Fatalf("missing helper data in output: %s", output)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		process, findErr := os.FindProcess(childPID)
		if findErr != nil || process.Signal(os.Signal(syscall.Signal(0))) != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	process, _ := os.FindProcess(childPID)
	if process != nil && process.Signal(os.Signal(syscall.Signal(0))) == nil {
		_ = process.Kill()
		t.Fatalf("renderer child %d survived parent exit", childPID)
	}
	if _, err := os.Stat(socketDir); !os.IsNotExist(err) {
		t.Fatalf("renderer socket directory still exists: %v", err)
	}
}

func TestRendererStopIsIdempotent(t *testing.T) {
	cleanupCalls := 0
	r := &Renderer{cleanup: func() { cleanupCalls++ }}

	if err := r.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := r.Stop(); err != nil {
		t.Fatal(err)
	}
	if cleanupCalls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", cleanupCalls)
	}
}

func TestRuntimeSourceIncludesReactPlugins(t *testing.T) {
	src := RuntimeSource(core.ModeDev)
	if !strings.Contains(src, "react-compiler") {
		t.Fatal("expected react-compiler plugin in react runtime source")
	}
	if !strings.Contains(src, "@babel/core") {
		t.Fatal("expected @babel/core import in react runtime source")
	}
	if strings.Contains(src, "svelte/compiler") {
		t.Fatal("did not expect react import in react-only runtime source")
	}
	if strings.Contains(src, "svelte-plugin") {
		t.Fatal("did not expect react plugin in react-only runtime source")
	}
}
