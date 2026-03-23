package process

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderChunkedFromDecoder_NDJSON(t *testing.T) {
	in := strings.NewReader("{\"head\":\"<title>x</title>\"}\n{\"html\":\"<p>y</p>\"}\n")
	dec := json.NewDecoder(in)
	var head, body string
	err := renderChunkedFromDecoder(dec,
		func(h string) error { head = h; return nil },
		func(b string) error { body = b; return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if head != "<title>x</title>" || body != "<p>y</p>" {
		t.Fatalf("got head=%q body=%q", head, body)
	}
}

func TestRenderChunkedFromDecoder_LegacySingleJSON(t *testing.T) {
	in := strings.NewReader("{\"head\":\"h\",\"html\":\"b\"}\n")
	dec := json.NewDecoder(in)
	var head, body string
	err := renderChunkedFromDecoder(dec,
		func(h string) error { head = h; return nil },
		func(b string) error { body = b; return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if head != "h" || body != "b" {
		t.Fatalf("got head=%q body=%q", head, body)
	}
}

func TestRenderChunkedFromDecoder_ErrorEnvelope(t *testing.T) {
	in := strings.NewReader("{\"error\":{\"message\":\"boom\"}}\n")
	dec := json.NewDecoder(in)
	err := renderChunkedFromDecoder(dec, func(string) error { return nil }, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected boom error, got %v", err)
	}
}
