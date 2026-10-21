package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type DialogProps struct {
	ID           string
	Title        string
	Description  string
	Trigger      g.Node
	TriggerLabel string
	Body         g.Node
	Footer       g.Node
}

func Dialog(props DialogProps) g.Node {
	id := props.ID
	if id == "" {
		id = ID("dialog")
	}
	trigger := props.Trigger
	if trigger == nil {
		label := props.TriggerLabel
		if label == "" {
			label = "Open dialog"
		}
		trigger = Button(ButtonProps{Attrs: []g.Node{g.Attr("@click", "show()")}}, g.Text(label))
	}
	header := []g.Node{h.Class("ui-dialog__header")}
	if props.Title != "" {
		header = append(header, h.H2(h.Class("ui-dialog__title"), h.ID(id+"-title"), g.Text(props.Title)))
	}
	header = append(header, h.Button(
		h.Type("button"),
		h.Class("ui-dialog__close"),
		g.Attr("@click", "close()"),
		g.Attr("aria-label", "Close dialog"),
		g.Text("×"),
	))
	inner := []g.Node{
		h.Class("ui-dialog__inner"),
		h.Div(header...),
	}
	if props.Description != "" {
		inner = append(inner, h.P(h.Class("ui-dialog__description"), g.Text(props.Description)))
	}
	if props.Body != nil {
		inner = append(inner, h.Div(h.Class("ui-dialog__body"), props.Body))
	}
	if props.Footer != nil {
		inner = append(inner, h.Footer(h.Class("ui-dialog__footer"), props.Footer))
	}
	dialog := []g.Node{
		h.Class("ui-dialog__panel"),
		h.ID(id),
		g.Attr("x-ref", "dialog"),
		g.Attr("aria-labelledby", id+"-title"),
		g.Attr("@close", "open = false"),
		g.Attr("@click", "closeOnBackdrop($event)"),
		h.Div(inner...),
	}
	return h.Div(
		h.Class("ui-dialog"),
		g.Attr("x-data", "uiDialog({id: "+quote(id)+"})"),
		g.Attr(":data-state", "open ? 'open' : 'closed'"),
		trigger,
		h.Dialog(dialog...),
	)
}
