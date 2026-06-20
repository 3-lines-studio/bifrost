package process

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestMarshalRenderRequestJSON_NilPropsEncoded(t *testing.T) {
	b, err := MarshalRenderRequestJSON("/p", nil)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["props"]; !ok {
		t.Fatalf("missing props in %s", string(b))
	}
	if string(got["props"]) != "null" {
		t.Fatalf("props: want null, got %s", string(got["props"]))
	}
}

func TestMarshalRenderRequestJSON_StructProps(t *testing.T) {
	type pageProps struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	b, err := MarshalRenderRequestJSON("/abs/ssr/page-ssr.js", pageProps{Name: "World", Count: 42})
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if string(got["path"]) != `"/abs/ssr/page-ssr.js"` {
		t.Errorf("unexpected path: %s", got["path"])
	}
	if string(got["props"]) != `{"name":"World","count":42}` {
		t.Errorf("unexpected props: %s", got["props"])
	}
}

func BenchmarkMarshalRenderRequestJSON(b *testing.B) {
	b.ReportAllocs()
	props := map[string]any{"name": "World", "count": 42}
	for i := 0; i < b.N; i++ {
		_, _ = MarshalRenderRequestJSON("/abs/ssr/page-ssr.js", props)
	}
}

func TestBunErrorJSON_UnmarshalNestedPosition(t *testing.T) {
	payload := `{
		"message": "Build failed",
		"stack": "",
		"errors": [
			{
				"message": "Could not resolve module",
				"stack": "",
				"position": {
					"file": "src/App.tsx",
					"line": 3,
					"column": 21,
					"lineText": "import \"invalid-import\";"
				},
				"specifier": "invalid-import",
				"referrer": "src/App.tsx"
			}
		]
	}`

	var got bunErrorJSON
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Message != "Build failed" {
		t.Errorf("expected message 'Build failed', got %q", got.Message)
	}
	if len(got.Errors) != 1 {
		t.Fatalf("expected 1 error detail, got %d", len(got.Errors))
	}
	detail := got.Errors[0]
	if detail.Position == nil {
		t.Fatal("expected nested position to be parsed")
	}
	if detail.Position.File != "src/App.tsx" {
		t.Errorf("expected file 'src/App.tsx', got %q", detail.Position.File)
	}
	if detail.Position.Line != 3 {
		t.Errorf("expected line 3, got %d", detail.Position.Line)
	}
	if detail.Position.Column != 21 {
		t.Errorf("expected column 21, got %d", detail.Position.Column)
	}
	if detail.Position.LineText != `import "invalid-import";` {
		t.Errorf("expected lineText 'import \"invalid-import\";', got %q", detail.Position.LineText)
	}
	if detail.Specifier != "invalid-import" {
		t.Errorf("expected specifier 'invalid-import', got %q", detail.Specifier)
	}
	if detail.Referrer != "src/App.tsx" {
		t.Errorf("expected referrer 'src/App.tsx', got %q", detail.Referrer)
	}
}

func TestBunErrorToStructured_PromotesSingleSubError(t *testing.T) {
	e := &bunErrorJSON{
		Message: "Build failed",
		Errors: []errorDetailJSON{
			{
				Message: "Could not resolve module",
				Position: &errorPositionJSON{
					File:     "src/App.tsx",
					Line:     3,
					Column:   21,
					LineText: `import "invalid-import";`,
				},
				Specifier: "invalid-import",
				Referrer:  "src/App.tsx",
			},
		},
	}

	se := bunErrorToStructured(e, "Build Error")
	if se == nil {
		t.Fatal("expected structured error")
	}
	if se.Message != "Build failed" {
		t.Errorf("expected parent message 'Build failed', got %q", se.Message)
	}
	if len(se.SubErrors) != 1 {
		t.Fatalf("expected 1 sub-error, got %d", len(se.SubErrors))
	}

	// The top-level structured error should expose the single sub-error's
	// location and specifier so the dev template can render them.
	if se.File != "src/App.tsx" {
		t.Errorf("expected parent File to be promoted to 'src/App.tsx', got %q", se.File)
	}
	if se.Line != 3 {
		t.Errorf("expected parent Line to be promoted to 3, got %d", se.Line)
	}
	if se.Column != 21 {
		t.Errorf("expected parent Column to be promoted to 21, got %d", se.Column)
	}
	if se.LineText != `import "invalid-import";` {
		t.Errorf("expected parent LineText to be promoted, got %q", se.LineText)
	}
	if se.Specifier != "invalid-import" {
		t.Errorf("expected parent Specifier to be promoted to 'invalid-import', got %q", se.Specifier)
	}
}

func TestFormatRenderError_WrappedStructuredErrorIsUnwrappable(t *testing.T) {
	se := formatRenderError(&bunErrorJSON{
		Message: "Render error",
		Stack:   "at Page (file.tsx:1:1)",
	})
	if se == nil {
		t.Fatal("expected structured error")
	}

	wrapped := fmt.Errorf("render failed: %w", se)
	var target *core.StructuredError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As failed to extract StructuredError through fmt.Errorf %w chain")
	}
	if target.Message != "Render error" {
		t.Errorf("expected unwrapped message 'Render error', got %q", target.Message)
	}
}
