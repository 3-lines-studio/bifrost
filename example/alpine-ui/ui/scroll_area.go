package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func ScrollArea(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-scroll-area"),
		g.Attr("data-slot", "scroll-area"),
		g.Attr("tabindex", "0"),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}
