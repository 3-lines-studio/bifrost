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
		Variant: VariantDefault,
		Size:    SizeLG,
		Attrs:   []g.Node{g.Attr("data-test", "save")},
	}, g.Text("Save")))
	for _, expected := range []string{`class="ui-button"`, `data-slot="button"`, `data-variant="default"`, `data-size="lg"`, `type="button"`, `data-test="save"`, `>Save</button>`} {
		if !strings.Contains(html, expected) {
			t.Fatalf("missing %q in %s", expected, html)
		}
	}
}

func TestCardAnatomy(t *testing.T) {
	html := render(t, Card(
		CardHeader(CardTitle(g.Text("Title")), CardDescription(g.Text("Description")), CardAction(Badge(VariantOutline, g.Text("Badge")))),
		CardContent(g.Text("Body")),
		CardFooter(g.Text("Footer")),
	))
	for _, expected := range []string{`data-slot="card"`, `data-slot="card-header"`, `data-slot="card-title"`, `data-slot="card-description"`, `data-slot="card-action"`, `data-slot="card-content"`, `data-slot="card-footer"`, `data-slot="badge"`} {
		if !strings.Contains(html, expected) {
			t.Fatalf("missing %q in %s", expected, html)
		}
	}
}

func TestDialogCompositionAndARIA(t *testing.T) {
	html := render(t, Dialog(
		DialogTrigger(Button(ButtonProps{}, g.Text("Open"))),
		DialogContent(
			DialogHeader(DialogTitle(g.Text("Delete?")), DialogDescription(g.Text("Context"))),
			DialogFooter(DialogClose(Button(ButtonProps{Variant: VariantOutline}, g.Text("Cancel"))), Button(ButtonProps{Variant: VariantDestructive}, g.Text("Delete"))),
		),
	))
	for _, expected := range []string{`data-slot="dialog"`, `data-slot="dialog-trigger"`, `data-slot="dialog-content"`, `data-slot="dialog-title"`, `data-slot="dialog-footer"`, `aria-labelledby`, `aria-describedby`, `show()`} {
		if !strings.Contains(html, expected) {
			t.Fatalf("missing %q in %s", expected, html)
		}
	}
}

func TestDropdownMenuComposition(t *testing.T) {
	html := render(t, DropdownMenu(
		DropdownMenuTrigger(Button(ButtonProps{}, g.Text("Actions"))),
		DropdownMenuContent(
			DropdownMenuLabel(g.Text("Account")),
			DropdownMenuItem(DropdownMenuItemProps{Href: "#"}, g.Text("Edit")),
			DropdownMenuSeparator(),
		),
	))
	for _, expected := range []string{`data-slot="dropdown-menu"`, `data-slot="dropdown-menu-trigger"`, `data-slot="dropdown-menu-content"`, `role="menu"`, `role="menuitem"`, `aria-haspopup="menu"`} {
		if !strings.Contains(html, expected) {
			t.Fatalf("missing %q in %s", expected, html)
		}
	}
}

func TestTabsComposition(t *testing.T) {
	html := render(t, Tabs(TabsProps{Value: "account"},
		TabsList(TabsTrigger(TabsTriggerProps{Value: "account"}, g.Text("Account")), TabsTrigger(TabsTriggerProps{Value: "billing"}, g.Text("Billing"))),
		TabsContent(TabsContentProps{Value: "account"}, g.Text("Account panel")),
		TabsContent(TabsContentProps{Value: "billing"}, g.Text("Billing panel")),
	))
	for _, expected := range []string{`data-slot="tabs"`, `role="tablist"`, `role="tab"`, `role="tabpanel"`, `aria-selected`, `aria-controls`} {
		if !strings.Contains(html, expected) {
			t.Fatalf("missing %q in %s", expected, html)
		}
	}
}

func TestToastRegionUsesStore(t *testing.T) {
	html := render(t, ToastRegion())
	if !strings.Contains(html, `$store.toasts.items`) || !strings.Contains(html, `aria-live="polite"`) {
		t.Fatalf("toast markup = %s", html)
	}
}

func TestAccordionComposition(t *testing.T) {
	html := render(t, Accordion(AccordionProps{Type: "single", DefaultValue: "one", Collapsible: true},
		AccordionItem(AccordionItemProps{Value: "one"},
			AccordionTrigger(AccordionTriggerProps{Value: "one"}, g.Text("One")),
			AccordionContent(AccordionContentProps{Value: "one"}, g.Text("Body")),
		),
	))
	for _, expected := range []string{`data-slot="accordion"`, `data-slot="accordion-item"`, `data-slot="accordion-trigger"`, `data-slot="accordion-content"`, `aria-expanded`, `role="region"`} {
		if !strings.Contains(html, expected) {
			t.Fatalf("missing %q in %s", expected, html)
		}
	}
}

func TestSheetComposition(t *testing.T) {
	html := render(t, Sheet(
		SheetTrigger(Button(ButtonProps{}, g.Text("Open"))),
		SheetContent(SheetContentProps{Side: "right"},
			SheetHeader(SheetTitle(g.Text("Filters")), SheetDescription(g.Text("Description"))),
			SheetFooter(SheetClose(Button(ButtonProps{}, g.Text("Close")))),
		),
	))
	for _, expected := range []string{`data-slot="sheet"`, `data-slot="sheet-trigger"`, `data-slot="sheet-content"`, `data-slot="sheet-title"`, `data-slot="sheet-footer"`, `data-side="right"`} {
		if !strings.Contains(html, expected) {
			t.Fatalf("missing %q in %s", expected, html)
		}
	}
}

func TestTooltipAndPopoverComposition(t *testing.T) {
	tooltip := render(t, Tooltip(TooltipTrigger(g.Text("?")), TooltipContent(g.Text("Help"))))
	for _, expected := range []string{`data-slot="tooltip"`, `data-slot="tooltip-trigger"`, `data-slot="tooltip-content"`, `role="tooltip"`} {
		if !strings.Contains(tooltip, expected) {
			t.Fatalf("missing %q in %s", expected, tooltip)
		}
	}
	popover := render(t, Popover(PopoverTrigger(g.Text("Open")), PopoverContent(g.Text("Body"))))
	for _, expected := range []string{`data-slot="popover"`, `data-slot="popover-trigger"`, `data-slot="popover-content"`, `role="dialog"`} {
		if !strings.Contains(popover, expected) {
			t.Fatalf("missing %q in %s", expected, popover)
		}
	}
}
