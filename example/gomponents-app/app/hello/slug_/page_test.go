package slug

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPageUsesPathParam(t *testing.T) {
	request := httptest.NewRequest("GET", "/hello/world", nil)
	request.SetPathValue("slug", "world")
	props, err := Load(request)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Page(props).Render(&output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Hello, world") {
		t.Fatalf("missing greeting:\n%s", output.String())
	}
}
