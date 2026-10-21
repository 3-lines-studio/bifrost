package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type ButtonProps struct {
	Variant  string
	Size     string
	Type     string
	Disabled bool
	Attrs    []g.Node
}

func Button(props ButtonProps, children ...g.Node) g.Node {
	variant := props.Variant
	if variant == "" {
		variant = Default
	}
	size := props.Size
	if size == "" {
		size = Medium
	}
	kind := props.Type
	if kind == "" {
		kind = "button"
	}
	nodes := []g.Node{
		h.Class("ui-button"),
		g.Attr("data-variant", variant),
		g.Attr("data-size", size),
		h.Type(kind),
	}
	nodes = append(nodes, props.Attrs...)
	if props.Disabled {
		nodes = append(nodes, h.Disabled())
	}
	nodes = append(nodes, children...)
	return h.Button(nodes...)
}

type LinkButtonProps struct {
	Href    string
	Variant string
	Size    string
	Attrs   []g.Node
}

func LinkButton(props LinkButtonProps, children ...g.Node) g.Node {
	variant := props.Variant
	if variant == "" {
		variant = Default
	}
	size := props.Size
	if size == "" {
		size = Medium
	}
	nodes := []g.Node{
		h.Href(props.Href),
		h.Class("ui-button"),
		g.Attr("data-variant", variant),
		g.Attr("data-size", size),
	}
	nodes = append(nodes, props.Attrs...)
	nodes = append(nodes, children...)
	return h.A(nodes...)
}
