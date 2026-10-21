package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type InputProps struct {
	Type        string
	Name        string
	ID          string
	Placeholder string
	Value       string
	Required    bool
	Disabled    bool
	Invalid     bool
	Attrs       []g.Node
}

func Input(props InputProps) g.Node {
	kind := props.Type
	if kind == "" {
		kind = "text"
	}
	nodes := []g.Node{
		h.Class("ui-input"),
		g.Attr("data-slot", "input"),
		h.Type(kind),
	}
	if props.Name != "" {
		nodes = append(nodes, h.Name(props.Name))
	}
	if props.ID != "" {
		nodes = append(nodes, h.ID(props.ID))
	}
	if props.Placeholder != "" {
		nodes = append(nodes, h.Placeholder(props.Placeholder))
	}
	if props.Value != "" {
		nodes = append(nodes, h.Value(props.Value))
	}
	if props.Required {
		nodes = append(nodes, h.Required())
	}
	if props.Disabled {
		nodes = append(nodes, h.Disabled())
	}
	if props.Invalid {
		nodes = append(nodes, g.Attr("aria-invalid", "true"))
	}
	nodes = append(nodes, props.Attrs...)
	return h.Input(nodes...)
}
