package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func RadioGroup(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-radio-group"),
		g.Attr("data-slot", "radio-group"),
		g.Attr("role", "radiogroup"),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

type RadioGroupItemProps struct {
	ID       string
	Name     string
	Value    string
	Checked  bool
	Disabled bool
}

func RadioGroupItem(props RadioGroupItemProps) g.Node {
	nodes := []g.Node{
		h.Class("ui-radio"),
		g.Attr("data-slot", "radio-group-item"),
		h.Type("radio"),
		h.Name(props.Name),
		h.Value(props.Value),
	}
	if props.ID != "" {
		nodes = append(nodes, h.ID(props.ID))
	}
	if props.Checked {
		nodes = append(nodes, h.Checked())
	}
	if props.Disabled {
		nodes = append(nodes, h.Disabled())
	}
	return h.Input(nodes...)
}
