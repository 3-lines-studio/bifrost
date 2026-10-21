package errordemo

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost"
)

func TestPageWrapsContentInLayout(t *testing.T) {
	var output bytes.Buffer
	props := Props{Message: "ok"}
	if err := Layout(Page(props), props).Render(&output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `class="status__panel"`) || !strings.Contains(output.String(), "ok") {
		t.Fatalf("layout output:\n%s", output.String())
	}
}

func TestErrorAndNotFoundRender(t *testing.T) {
	var output bytes.Buffer
	if err := Error(bifrost.Status(503, nil), httptest.NewRequest("GET", "/", nil)).Render(&output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Custom error") {
		t.Fatalf("error output:\n%s", output.String())
	}
	output.Reset()
	if err := NotFound(httptest.NewRequest("GET", "/missing", nil)).Render(&output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "/missing") {
		t.Fatalf("not-found output:\n%s", output.String())
	}
}
