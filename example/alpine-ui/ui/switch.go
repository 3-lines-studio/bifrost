package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type SwitchProps struct {
	ID       string
	Name     string
	Checked  bool
	Disabled bool
	Attrs    []g.Node
}

func Switch(props SwitchProps) g.Node {
	nodes := []g.Node{
		h.Class("ui-switch"),
		g.Attr("data-slot", "switch"),
		h.Type("checkbox"),
		g.Attr("role", "switch"),
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
