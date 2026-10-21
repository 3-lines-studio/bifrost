package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func Skeleton(attrs ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-skeleton"), g.Attr("data-slot", "skeleton")}
	nodes = append(nodes, attrs...)
	return h.Div(nodes...)
}
