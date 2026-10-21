package ui

import (
	"bytes"
	"strings"
	"testing"

	g "maragu.dev/gomponents"
)

func render(t *testing.T, node g.Node) string {
	t.Helper()
	var output bytes.Buffer
	if err := node.Render(&output); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestButtonVariantsAndAttributes(t *testing.T) {
	html := render(t, Button(ButtonProps{
		Variant: Primary,
		Size:    Large,
		Attrs:   []g.Node{g.Attr("data-test", "save")},
	}, g.Text("Save")))
	for _, expected := range []string{`class="ui-button"`, `data-variant="primary"`, `data-size="lg"`, `type="button"`, `data-test="save"`, `>Save</button>`} {
		if !strings.Contains(html, expected) {
			t.Fatalf("missing %q in %s", expected, html)
		}
	}
}

func TestDialogHasNativeMarkupAndARIA(t *testing.T) {
	html := render(t, Dialog(DialogProps{
		ID:           "delete",
		Title:        "Delete?",
		TriggerLabel: "Open",
		Body:         g.Text("Context"),
	}))
	for _, expected := range []string{`<dialog`, `id="delete"`, `aria-labelledby="delete-title"`, `id="delete-title"`, `Open`} {
		if !strings.Contains(html, expected) {
			t.Fatalf("missing %q in %s", expected, html)
		}
	}
}

func TestMenuLinksItsTriggerAndPanel(t *testing.T) {
	html := render(t, Menu(MenuProps{
		Label: "Actions",
		Items: []MenuItem{{Label: "Edit", Href: "#"}},
	}))
	if !strings.Contains(html, `aria-controls=`) || !strings.Contains(html, `role="menu"`) || !strings.Contains(html, `role="menuitem"`) {
		t.Fatalf("menu markup = %s", html)
	}
}

func TestToastRegionUsesStore(t *testing.T) {
	html := render(t, ToastRegion())
	if !strings.Contains(html, `$store.toasts.items`) || !strings.Contains(html, `aria-live="polite"`) {
		t.Fatalf("toast markup = %s", html)
	}
}
