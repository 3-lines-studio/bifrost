package app

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPageRendersTypedComponents(t *testing.T) {
	props, err := Load(httptest.NewRequest("GET", "/?name=Ada", nil))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Page(props).Render(&output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, expected := range []string{
		"Ada",
		"Parent passes variables",
		"Card 01",
		`x-data="{ open: false }"`,
		`@click="count++"`,
		"bg-slate-950",
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("missing %q in:\n%s", expected, html)
		}
	}
}
