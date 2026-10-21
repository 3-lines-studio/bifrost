package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func Breadcrumb(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-breadcrumb"), g.Attr("data-slot", "breadcrumb"), g.Attr("aria-label", "breadcrumb")}
	nodes = append(nodes, children...)
	return h.Nav(nodes...)
}

func BreadcrumbList(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-breadcrumb__list"), g.Attr("data-slot", "breadcrumb-list")}
	nodes = append(nodes, children...)
	return h.Ol(nodes...)
}

func BreadcrumbItem(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-breadcrumb__item"), g.Attr("data-slot", "breadcrumb-item")}
	nodes = append(nodes, children...)
	return h.Li(nodes...)
}

func BreadcrumbLink(href string, children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-breadcrumb__link"),
		g.Attr("data-slot", "breadcrumb-link"),
		h.Href(href),
	}
	nodes = append(nodes, children...)
	return h.A(nodes...)
}

func BreadcrumbPage(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-breadcrumb__page"),
		g.Attr("data-slot", "breadcrumb-page"),
		g.Attr("aria-current", "page"),
	}
	nodes = append(nodes, children...)
	return h.Span(nodes...)
}

func BreadcrumbSeparator(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-breadcrumb__separator"),
		g.Attr("data-slot", "breadcrumb-separator"),
		g.Attr("role", "presentation"),
		g.Attr("aria-hidden", "true"),
	}
	if len(children) == 0 {
		children = []g.Node{g.Text("/")}
	}
	nodes = append(nodes, children...)
	return h.Li(nodes...)
}
