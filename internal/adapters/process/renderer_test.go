package process

import (
	"strings"
	"testing"
)

func TestFormatRenderError_CompileError(t *testing.T) {
	e := &renderErrJSON{
		Message: "Build failed - Expected whitespace https://svelte.dev/e/expected_whitespace (expected_whitespace)",
		Stack:   "",
	}

	err := formatRenderError(e)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "expected_whitespace") {
		t.Errorf("expected 'expected_whitespace' in error, got: %s", errStr)
	}
	if !strings.Contains(errStr, "svelte.dev/e/expected_whitespace") {
		t.Errorf("expected svelte.dev URL in error, got: %s", errStr)
	}
}

func TestFormatRenderError_WithStack(t *testing.T) {
	e := &renderErrJSON{
		Message: "Build failed",
		Stack:   "at compile (/src/svelte/compiler.js:123:4)",
	}

	err := formatRenderError(e)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "Build failed") {
		t.Errorf("expected 'Build failed' in error, got: %s", errStr)
	}
	if !strings.Contains(errStr, "at compile") {
		t.Errorf("expected stack trace in error, got: %s", errStr)
	}
}

func TestFormatRenderError_WithSubErrors(t *testing.T) {
	e := &renderErrJSON{
		Message: "Build failed",
		Errors: []struct {
			Message string `json:"message"`
			Stack   string `json:"stack"`
		}{
			{Message: "Expected whitespace", Stack: "line 2"},
			{Message: "Unexpected token", Stack: "line 3"},
		},
	}

	err := formatRenderError(e)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errStr := err.Error()
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
	err := formatRenderError(nil)
	if err != nil {
		t.Fatalf("expected nil for nil input, got: %v", err)
	}
}
