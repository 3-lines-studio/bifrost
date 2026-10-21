package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type FieldProps struct {
	Label   string
	ID      string
	Hint    string
	Error   string
	Control g.Node
}

func Field(props FieldProps) g.Node {
	nodes := []g.Node{h.Class("ui-field")}
	if props.Label != "" {
		nodes = append(nodes, h.Label(h.For(props.ID), h.Class("ui-field__label"), g.Text(props.Label)))
	}
	nodes = append(nodes, props.Control)
	if props.Error != "" {
		nodes = append(nodes, h.P(h.Class("ui-field__error"), g.Attr("id", props.ID+"-error"), g.Text(props.Error)))
	} else if props.Hint != "" {
		nodes = append(nodes, h.P(h.Class("ui-field__hint"), g.Attr("id", props.ID+"-hint"), g.Text(props.Hint)))
	}
	return h.Div(nodes...)
}

type InputProps struct {
	Type        string
	Name        string
	ID          string
	Placeholder string
	Value       string
	Required    bool
	Disabled    bool
	Attrs       []g.Node
}

func Input(props InputProps) g.Node {
	kind := props.Type
	if kind == "" {
		kind = "text"
	}
	nodes := []g.Node{h.Class("ui-input"), h.Type(kind)}
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
	nodes = append(nodes, props.Attrs...)
	return h.Input(nodes...)
}
