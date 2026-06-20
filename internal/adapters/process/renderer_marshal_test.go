package process

import (
	"encoding/json"
	"testing"
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

func BenchmarkMarshalRenderRequestJSON(b *testing.B) {
	b.ReportAllocs()
	props := map[string]any{"name": "World", "count": 42}
	for i := 0; i < b.N; i++ {
		_, _ = MarshalRenderRequestJSON("/abs/ssr/page-ssr.js", props)
	}
}
