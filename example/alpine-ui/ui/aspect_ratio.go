package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type AspectRatioProps struct {
	Ratio string
}

func AspectRatio(props AspectRatioProps, children ...g.Node) g.Node {
	ratio := props.Ratio
	if ratio == "" {
		ratio = "16 / 9"
	}
	nodes := []g.Node{
		h.Class("ui-aspect-ratio"),
		g.Attr("data-slot", "aspect-ratio"),
		g.Attr("style", "aspect-ratio: "+ratio),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}
