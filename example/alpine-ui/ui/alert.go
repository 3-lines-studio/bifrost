package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func Alert(variant string, children ...g.Node) g.Node {
	if variant == "" {
		variant = VariantDefault
	}
	nodes := []g.Node{
		h.Class("ui-alert"),
		g.Attr("data-slot", "alert"),
		g.Attr("data-variant", variant),
		g.Attr("role", "alert"),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func AlertTitle(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-alert__title"), g.Attr("data-slot", "alert-title")}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func AlertDescription(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-alert__description"), g.Attr("data-slot", "alert-description")}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}
