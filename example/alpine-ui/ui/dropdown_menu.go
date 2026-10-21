package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func DropdownMenu(children ...g.Node) g.Node {
	id := ID("dropdown-menu")
	nodes := []g.Node{
		h.Class("ui-dropdown-menu"),
		g.Attr("data-slot", "dropdown-menu"),
		g.Attr("x-data", "uiMenu({id: "+quote(id)+"})"),
		g.Attr("@click.outside", "close(false)"),
		g.Attr("@keydown.escape.stop", "close()"),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func DropdownMenuTrigger(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-dropdown-menu__trigger"),
		g.Attr("data-slot", "dropdown-menu-trigger"),
		g.Attr("aria-haspopup", "menu"),
		g.Attr(":aria-expanded", "open"),
		g.Attr(":aria-controls", "id"),
		g.Attr("x-ref", "trigger"),
		g.Attr("@click", "toggle()"),
		g.Attr("@keydown.down.prevent", "openAndFocus()"),
	}
	nodes = append(nodes, children...)
	return h.Span(nodes...)
}

func DropdownMenuContent(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-dropdown-menu__panel"),
		g.Attr("data-slot", "dropdown-menu-content"),
		g.Attr("role", "menu"),
		g.Attr("x-ref", "menu"),
		g.Attr(":id", "id"),
		g.Attr("x-show", "open"),
		g.Attr("x-cloak", ""),
		g.Attr(":data-state", "open ? 'open' : 'closed'"),
		g.Attr("@keydown", "onKeydown($event)"),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

type DropdownMenuItemProps struct {
	Href     string
	Disabled bool
}

func DropdownMenuItem(props DropdownMenuItemProps, children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-dropdown-menu__item"),
		g.Attr("data-slot", "dropdown-menu-item"),
		g.Attr("role", "menuitem"),
		g.Attr("tabindex", "-1"),
		g.Attr("@click", "close(false)"),
	}
	if props.Disabled {
		nodes = append(nodes, g.Attr("aria-disabled", "true"), g.Attr("data-disabled", ""))
	}
	nodes = append(nodes, children...)
	if props.Href != "" {
		nodes = append([]g.Node{h.Href(props.Href)}, nodes...)
		return h.A(nodes...)
	}
	nodes = append([]g.Node{h.Type("button")}, nodes...)
	return h.Button(nodes...)
}

func DropdownMenuLabel(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-dropdown-menu__label"),
		g.Attr("data-slot", "dropdown-menu-label"),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func DropdownMenuSeparator() g.Node {
	return h.Div(
		h.Class("ui-dropdown-menu__separator"),
		g.Attr("data-slot", "dropdown-menu-separator"),
		g.Attr("role", "separator"),
	)
}
