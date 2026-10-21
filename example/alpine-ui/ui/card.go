package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type CardProps struct {
	Title       string
	Description string
	Body        g.Node
	Footer      g.Node
	Attrs       []g.Node
}

func Card(props CardProps) g.Node {
	nodes := []g.Node{h.Class("ui-card")}
	nodes = append(nodes, props.Attrs...)
	if props.Title != "" || props.Description != "" {
		header := []g.Node{h.Class("ui-card__header")}
		if props.Title != "" {
			header = append(header, h.H2(h.Class("ui-card__title"), g.Text(props.Title)))
		}
		if props.Description != "" {
			header = append(header, h.P(h.Class("ui-card__description"), g.Text(props.Description)))
		}
		nodes = append(nodes, h.Header(header...))
	}
	if props.Body != nil {
		nodes = append(nodes, h.Div(h.Class("ui-card__body"), props.Body))
	}
	if props.Footer != nil {
		nodes = append(nodes, h.Footer(h.Class("ui-card__footer"), props.Footer))
	}
	return h.Article(nodes...)
}

func Badge(variant string, children ...g.Node) g.Node {
	if variant == "" {
		variant = Default
	}
	nodes := []g.Node{h.Class("ui-badge"), g.Attr("data-variant", variant)}
	nodes = append(nodes, children...)
	return h.Span(nodes...)
}
