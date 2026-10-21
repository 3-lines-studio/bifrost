package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type CollapsibleProps struct {
	DefaultOpen bool
}

func Collapsible(props CollapsibleProps, children ...g.Node) g.Node {
	id := ID("collapsible")
	open := "false"
	if props.DefaultOpen {
		open = "true"
	}
	nodes := []g.Node{
		h.Class("ui-collapsible"),
		g.Attr("data-slot", "collapsible"),
		g.Attr("x-data", "uiCollapsible({id: "+quote(id)+", open: "+open+"})"),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func CollapsibleTrigger(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-collapsible__trigger"),
		g.Attr("data-slot", "collapsible-trigger"),
		g.Attr(":aria-expanded", "open"),
		g.Attr(":aria-controls", "id + '-content'"),
		g.Attr(":data-state", "open ? 'open' : 'closed'"),
		g.Attr("@click", "toggle()"),
	}
	nodes = append(nodes, children...)
	return h.Span(nodes...)
}

func CollapsibleContent(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-collapsible__content"),
		g.Attr("data-slot", "collapsible-content"),
		g.Attr(":id", "id + '-content'"),
		g.Attr("x-show", "open"),
		g.Attr("x-cloak", ""),
		g.Attr(":data-state", "open ? 'open' : 'closed'"),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}
