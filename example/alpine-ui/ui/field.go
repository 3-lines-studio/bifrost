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
	nodes := []g.Node{h.Class("ui-field"), g.Attr("data-slot", "field")}
	if props.Label != "" {
		nodes = append(nodes, Label(LabelProps{For: props.ID}, g.Text(props.Label)))
	}
	nodes = append(nodes, props.Control)
	if props.Error != "" {
		nodes = append(nodes, h.P(h.Class("ui-field__error"), h.ID(props.ID+"-error"), g.Text(props.Error)))
	} else if props.Hint != "" {
		nodes = append(nodes, h.P(h.Class("ui-field__hint"), h.ID(props.ID+"-hint"), g.Text(props.Hint)))
	}
	return h.Div(nodes...)
}
