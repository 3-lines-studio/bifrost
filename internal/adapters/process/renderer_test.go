package process

import (
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestFormatRenderError_CompileError(t *testing.T) {
	e := &bunErrorJSON{
		Message: "Build failed - Expected whitespace https://svelte.dev/e/expected_whitespace (expected_whitespace)",
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
	if !strings.Contains(errStr, "svelte.dev/e/expected_whitespace") {
		t.Errorf("expected svelte.dev URL in error, got: %s", errStr)
	}
}

func TestFormatRenderError_WithStack(t *testing.T) {
	e := &bunErrorJSON{
		Message: "Build failed",
		Stack:   "at compile (/src/svelte/compiler.js:123:4)",
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
					File:     "src/App.svelte",
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
	if sub.File != "src/App.svelte" {
		t.Errorf("expected file 'src/App.svelte', got %q", sub.File)
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

func TestRuntimeSourceIncludesOnlyReactPluginsForReact(t *testing.T) {
	src := RuntimeSource(core.ModeDev, core.FrameworkReact)
	if !strings.Contains(src, "react-compiler") {
		t.Fatal("expected react-compiler plugin in react runtime source")
	}
	if !strings.Contains(src, "@babel/core") {
		t.Fatal("expected @babel/core import in react runtime source")
	}
	if strings.Contains(src, "svelte/compiler") {
		t.Fatal("did not expect svelte/compiler import in react-only runtime source")
	}
	if strings.Contains(src, "svelte-plugin") {
		t.Fatal("did not expect svelte plugin in react-only runtime source")
	}
}

func TestRuntimeSourceIncludesOnlySveltePluginsForSvelte(t *testing.T) {
	src := RuntimeSource(core.ModeDev, core.FrameworkSvelte)
	if !strings.Contains(src, "svelte/compiler") {
		t.Fatal("expected svelte/compiler import in svelte runtime source")
	}
	if !strings.Contains(src, "svelte-plugin") {
		t.Fatal("expected svelte plugin in svelte runtime source")
	}
	if strings.Contains(src, "@babel/core") {
		t.Fatal("did not expect @babel/core import in svelte-only runtime source")
	}
	if strings.Contains(src, "babel-plugin-react-compiler") {
		t.Fatal("did not expect babel-plugin-react-compiler import in svelte-only runtime source")
	}
}

func TestRuntimeSourceIncludesBothPluginsForMixedFrameworks(t *testing.T) {
	src := RuntimeSource(core.ModeDev, core.FrameworkReact, core.FrameworkSvelte)
	if !strings.Contains(src, "react-compiler") {
		t.Fatal("expected react-compiler plugin in mixed runtime source")
	}
	if !strings.Contains(src, "svelte-plugin") {
		t.Fatal("expected svelte plugin in mixed runtime source")
	}
}

func TestRuntimeSourceOmitsBothFrameworkPluginsWhenEmpty(t *testing.T) {
	src := RuntimeSource(core.ModeDev)
	if strings.Contains(src, "@babel/core") {
		t.Fatal("did not expect @babel/core import when no frameworks specified")
	}
	if strings.Contains(src, "svelte/compiler") {
		t.Fatal("did not expect svelte/compiler import when no frameworks specified")
	}
}
