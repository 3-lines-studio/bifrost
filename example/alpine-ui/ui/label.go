package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type LabelProps struct {
	For   string
	Attrs []g.Node
}

func Label(props LabelProps, children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-label"),
		g.Attr("data-slot", "label"),
	}
	if props.For != "" {
		nodes = append(nodes, h.For(props.For))
	}
	nodes = append(nodes, props.Attrs...)
	nodes = append(nodes, children...)
	return h.Label(nodes...)
}
