package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func Form(attrs []g.Node, children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-form"), g.Attr("data-slot", "form")}
	nodes = append(nodes, attrs...)
	nodes = append(nodes, children...)
	return h.Form(nodes...)
}

func FormItem(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-form-item"), g.Attr("data-slot", "form-item")}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func FormLabel(forID string, children ...g.Node) g.Node {
	return Label(LabelProps{For: forID, Attrs: []g.Node{g.Attr("data-slot", "form-label")}}, children...)
}

func FormControl(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-form-control"), g.Attr("data-slot", "form-control")}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func FormDescription(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-form-description"), g.Attr("data-slot", "form-description")}
	nodes = append(nodes, children...)
	return h.P(nodes...)
}

func FormMessage(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-form-message"), g.Attr("data-slot", "form-message")}
	nodes = append(nodes, children...)
	return h.P(nodes...)
}
