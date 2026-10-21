package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type TextareaProps struct {
	Name        string
	ID          string
	Placeholder string
	Value       string
	Rows        string
	Disabled    bool
	Invalid     bool
	Attrs       []g.Node
}

func Textarea(props TextareaProps) g.Node {
	nodes := []g.Node{
		h.Class("ui-textarea"),
		g.Attr("data-slot", "textarea"),
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
	if props.Rows != "" {
		nodes = append(nodes, h.Rows(props.Rows))
	}
	if props.Disabled {
		nodes = append(nodes, h.Disabled())
	}
	if props.Invalid {
		nodes = append(nodes, g.Attr("aria-invalid", "true"))
	}
	nodes = append(nodes, props.Attrs...)
	if props.Value != "" {
		nodes = append(nodes, g.Text(props.Value))
	}
	return h.Textarea(nodes...)
}
