package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type ToggleProps struct {
	Variant  string
	Size     string
	Pressed  bool
	Disabled bool
	Attrs    []g.Node
}

func Toggle(props ToggleProps, children ...g.Node) g.Node {
	return toggleNode("toggle", props, children...)
}

func ToggleGroup(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-toggle-group"),
		g.Attr("data-slot", "toggle-group"),
		g.Attr("role", "group"),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func ToggleGroupItem(props ToggleProps, children ...g.Node) g.Node {
	return toggleNode("toggle-group-item", props, children...)
}

func toggleNode(slot string, props ToggleProps, children ...g.Node) g.Node {
	variant := props.Variant
	if variant == "" {
		variant = VariantDefault
	}
	size := props.Size
	if size == "" {
		size = SizeDefault
	}
	state := "off"
	pressed := "false"
	if props.Pressed {
		state = "on"
		pressed = "true"
	}
	nodes := []g.Node{
		h.Class("ui-toggle"),
		g.Attr("data-slot", slot),
		g.Attr("data-variant", variant),
		g.Attr("data-size", size),
		g.Attr("data-state", state),
		g.Attr("type", "button"),
		g.Attr("aria-pressed", pressed),
	}
	if props.Disabled {
		nodes = append(nodes, h.Disabled())
	}
	nodes = append(nodes, props.Attrs...)
	nodes = append(nodes, children...)
	return h.Button(nodes...)
}
