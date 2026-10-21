package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func Badge(variant string, children ...g.Node) g.Node {
	if variant == "" {
		variant = VariantDefault
	}
	nodes := []g.Node{
		h.Class("ui-badge"),
		g.Attr("data-slot", "badge"),
		g.Attr("data-variant", variant),
	}
	nodes = append(nodes, children...)
	return h.Span(nodes...)
}
