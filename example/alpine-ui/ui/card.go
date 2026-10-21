package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func Card(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-card"), g.Attr("data-slot", "card")}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func CardHeader(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-card__header"), g.Attr("data-slot", "card-header")}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func CardTitle(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-card__title"), g.Attr("data-slot", "card-title")}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func CardDescription(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-card__description"), g.Attr("data-slot", "card-description")}
	nodes = append(nodes, children...)
	return h.P(nodes...)
}

func CardAction(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-card__action"), g.Attr("data-slot", "card-action")}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func CardContent(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-card__content"), g.Attr("data-slot", "card-content")}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func CardFooter(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-card__footer"), g.Attr("data-slot", "card-footer")}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}
