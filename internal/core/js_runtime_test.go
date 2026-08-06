package core

import "testing"

func TestNormalizeJSRuntime(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{value: "", want: JSRuntimeQuickJS},
		{value: "sobek", want: JSRuntimeSobek},
		{value: "SOBEK", want: JSRuntimeSobek},
		{value: "bun", want: JSRuntimeBun},
		{value: " BUN ", want: JSRuntimeBun},
		{value: "quickjs", want: JSRuntimeQuickJS},
		{value: "QUICKJS", want: JSRuntimeQuickJS},
		{value: "modernc", want: JSRuntimeModernc},
		{value: "MODERNC", want: JSRuntimeModernc},
	}
	for _, test := range tests {
		if got := NormalizeJSRuntime(test.value); got != test.want {
			t.Errorf("NormalizeJSRuntime(%q) = %q, want %q", test.value, got, test.want)
		}
	}
}
