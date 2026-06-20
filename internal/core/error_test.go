package core

import (
	"bytes"
	"strings"
	"testing"
)

func TestStructuredError_ErrorIncludesSubErrors(t *testing.T) {
	e := &StructuredError{
		ErrorType: "Build Error",
		Message:   "Build failed",
		SubErrors: []StructuredError{
			{Message: "Expected whitespace", File: "src/App.svelte", Line: 2, Column: 1},
		},
	}
	errStr := e.Error()
	if !strings.Contains(errStr, "Expected whitespace") {
		t.Errorf("expected sub-error message in Error(), got: %s", errStr)
	}
	if !strings.Contains(errStr, "src/App.svelte:2:1") {
		t.Errorf("expected sub-error location in Error(), got: %s", errStr)
	}
}

func TestErrorTemplate_RendersSubErrorLocationAndSnippet(t *testing.T) {
	data := ErrorData{
		IsDev: true,
		Structured: &StructuredError{
			ErrorType: "Build Error",
			Message:   "Build failed",
			File:      "src/App.tsx",
			Line:      3,
			Column:    21,
			LineText:  `import "invalid-import";`,
			SubErrors: []StructuredError{
				{
					Message:  "Could not resolve module",
					File:     "src/App.tsx",
					Line:     3,
					Column:   21,
					LineText: `import "invalid-import";`,
				},
			},
		},
		CodeSnippet: `import "invalid-import";`,
		NextSteps:   []string{"Check that import path is valid.", "Try running: bun install"},
	}

	var buf bytes.Buffer
	if err := ErrorTemplate.Execute(&buf, data); err != nil {
		t.Fatalf("template execution failed: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "Build Error") {
		t.Errorf("expected error type badge in output")
	}
	if !strings.Contains(out, `<div class="location"><strong>src/App.tsx</strong>:3:21</div>`) {
		t.Errorf("expected file:line:column in hero location, got: %s", out)
	}
	if !strings.Contains(out, "import") || !strings.Contains(out, "invalid-import") {
		t.Errorf("expected code snippet in output, got: %s", out)
	}
	if !strings.Contains(out, "</ul>") {
		t.Errorf("expected next-steps <ul> to be closed with </ul>; unbalanced tags in output")
	}
}

func TestErrorTemplate_ProductionHidesDetails(t *testing.T) {
	data := ErrorData{
		Message: "something went wrong",
		IsDev:   false,
	}

	var buf bytes.Buffer
	if err := ErrorTemplate.Execute(&buf, data); err != nil {
		t.Fatalf("template execution failed: %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "something went wrong") {
		t.Errorf("production error page should not leak raw error message")
	}
	if !strings.Contains(out, "An error occurred while processing your request") {
		t.Errorf("expected generic production message in output")
	}
}
