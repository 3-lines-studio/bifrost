package renderproc

import (
	"encoding/json"
	"testing"
)

func TestRenderPayloadJSONKeepsEncodedProps(t *testing.T) {
	body, err := renderPayloadJSON("ssr/app.js", json.RawMessage(`{"title":"Hello \u003cworld\u003e"}`))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"entry":"ssr/app.js","props":{"title":"Hello \u003cworld\u003e"}}`
	if string(body) != want {
		t.Fatalf("payload = %s", body)
	}
	body, err = renderPayloadJSON(`ssr/a"b.js`, nil)
	if err != nil {
		t.Fatal(err)
	}
	want = `{"entry":"ssr/a\"b.js","props":{}}`
	if string(body) != want {
		t.Fatalf("payload = %s", body)
	}
}
