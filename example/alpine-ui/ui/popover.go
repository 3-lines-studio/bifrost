package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func Popover(children ...g.Node) g.Node {
	id := ID("popover")
	nodes := []g.Node{
		h.Class("ui-popover"),
		g.Attr("data-slot", "popover"),
		g.Attr("x-data", "uiPopover({id: "+quote(id)+"})"),
		g.Attr("@click.outside", "close()"),
		g.Attr("@keydown.escape", "close()"),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func PopoverTrigger(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-popover__trigger"),
		g.Attr("data-slot", "popover-trigger"),
		g.Attr("aria-haspopup", "dialog"),
		g.Attr(":aria-expanded", "open"),
		g.Attr(":aria-controls", "id"),
		g.Attr("@click", "toggle()"),
	}
	nodes = append(nodes, children...)
	return h.Span(nodes...)
}

func PopoverContent(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-popover__content"),
		g.Attr("data-slot", "popover-content"),
		g.Attr("role", "dialog"),
		g.Attr(":id", "id"),
		g.Attr("x-show", "open"),
		g.Attr("x-cloak", ""),
		g.Attr(":data-state", "open ? 'open' : 'closed'"),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}
