package core

import "testing"

func TestNormalizeJSRuntime(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{value: "", want: JSRuntimeSobek},
		{value: "sobek", want: JSRuntimeSobek},
		{value: "SOBEK", want: JSRuntimeSobek},
		{value: "bun", want: JSRuntimeBun},
		{value: " BUN ", want: JSRuntimeBun},
	}
	for _, test := range tests {
		if got := NormalizeJSRuntime(test.value); got != test.want {
			t.Errorf("NormalizeJSRuntime(%q) = %q, want %q", test.value, got, test.want)
		}
	}
}
