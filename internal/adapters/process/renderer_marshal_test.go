package process

import (
	"encoding/json"
	"testing"
)

func TestMarshalRenderRequestJSON_StreamBody(t *testing.T) {
	b, err := MarshalRenderRequestJSON("/abs/ssr/page-ssr.js", map[string]any{"k": 1}, true)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["streamBody"]; !ok {
		t.Fatalf("missing streamBody in %s", string(b))
	}
	var streamBody bool
	if err := json.Unmarshal(got["streamBody"], &streamBody); err != nil || !streamBody {
		t.Fatalf("streamBody: want true, got %q err %v", string(got["streamBody"]), err)
	}
}

func TestMarshalRenderRequestJSON_NoStreamBodyOmitted(t *testing.T) {
	b, err := MarshalRenderRequestJSON("/p", map[string]any{}, false)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["streamBody"]; ok {
		t.Fatalf("expected streamBody omitted, got %s", string(b))
	}
}
