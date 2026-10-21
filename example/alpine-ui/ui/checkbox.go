package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type CheckboxProps struct {
	ID       string
	Name     string
	Checked  bool
	Disabled bool
	Attrs    []g.Node
}

func Checkbox(props CheckboxProps) g.Node {
	nodes := []g.Node{
		h.Class("ui-checkbox"),
		g.Attr("data-slot", "checkbox"),
		h.Type("checkbox"),
	}
	if props.ID != "" {
		nodes = append(nodes, h.ID(props.ID))
	}
	if props.Name != "" {
		nodes = append(nodes, h.Name(props.Name))
	}
	if props.Checked {
		nodes = append(nodes, h.Checked())
	}
	if props.Disabled {
		nodes = append(nodes, h.Disabled())
	}
	nodes = append(nodes, props.Attrs...)
	return h.Input(nodes...)
}
