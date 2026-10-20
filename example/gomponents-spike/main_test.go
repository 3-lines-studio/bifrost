package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPageRendersNestedComponentsAndAlpine(t *testing.T) {
	var output bytes.Buffer
	if err := Page(demoPage()).Render(&output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, expected := range []string{
		"Ada Lovelace",
		"Parent passes variables",
		"Default summary",
		`x-data="{ open: false }"`,
		`@click.outside="open = false"`,
		`x-data="{ count: 0 }"`,
		`@click="count++"`,
		"bg-slate-950",
		"<title>Gomponents spike</title>",
		"Gomponents + Alpine + Tailwind",
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("missing %q in:\n%s", expected, html)
		}
	}
	if strings.Contains(html, "<no value>") {
		t.Fatalf("unexpected template value:\n%s", html)
	}
}

func TestCardDefaults(t *testing.T) {
	var output bytes.Buffer
	if err := Card(CardProps{Title: "Only a title"}).Render(&output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if !strings.Contains(html, "Default summary") || !strings.Contains(html, "Component") {
		t.Fatalf("defaults missing:\n%s", html)
	}
}
