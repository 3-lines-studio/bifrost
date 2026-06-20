package process

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderFromDecoder_NDJSON(t *testing.T) {
	in := strings.NewReader("{\"head\":\"<title>x</title>\"}\n{\"html\":\"<p>y</p>\"}\n")
	dec := json.NewDecoder(in)
	head, body, err := renderFromDecoder(dec)
	if err != nil {
		t.Fatal(err)
	}
	if head != "<title>x</title>" || body != "<p>y</p>" {
		t.Fatalf("got head=%q body=%q", head, body)
	}
}

func TestRenderFromDecoder_LegacySingleJSON(t *testing.T) {
	in := strings.NewReader("{\"head\":\"h\",\"html\":\"b\"}\n")
	dec := json.NewDecoder(in)
	head, body, err := renderFromDecoder(dec)
	if err != nil {
		t.Fatal(err)
	}
	if head != "h" || body != "b" {
		t.Fatalf("got head=%q body=%q", head, body)
	}
}

func TestRenderFromDecoder_ErrorEnvelope(t *testing.T) {
	in := strings.NewReader("{\"error\":{\"message\":\"boom\"}}\n")
	dec := json.NewDecoder(in)
	_, _, err := renderFromDecoder(dec)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected boom error, got %v", err)
	}
}
