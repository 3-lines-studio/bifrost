package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func Pagination(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-pagination"),
		g.Attr("data-slot", "pagination"),
		g.Attr("role", "navigation"),
		g.Attr("aria-label", "pagination"),
	}
	nodes = append(nodes, children...)
	return h.Nav(nodes...)
}

func PaginationContent(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-pagination__content"), g.Attr("data-slot", "pagination-content")}
	nodes = append(nodes, children...)
	return h.Ul(nodes...)
}

func PaginationItem(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-pagination__item"), g.Attr("data-slot", "pagination-item")}
	nodes = append(nodes, children...)
	return h.Li(nodes...)
}

type PaginationLinkProps struct {
	Href     string
	IsActive bool
	Size     string
}

func PaginationLink(props PaginationLinkProps, children ...g.Node) g.Node {
	size := props.Size
	if size == "" {
		size = SizeDefault
	}
	nodes := []g.Node{
		h.Class("ui-pagination__link"),
		g.Attr("data-slot", "pagination-link"),
		g.Attr("data-size", size),
		h.Href(props.Href),
	}
	if props.IsActive {
		nodes = append(nodes, g.Attr("aria-current", "page"))
	}
	nodes = append(nodes, children...)
	return h.A(nodes...)
}

func PaginationPrevious(href string) g.Node {
	return PaginationLink(PaginationLinkProps{Href: href}, g.Text("Anterior"))
}

func PaginationNext(href string) g.Node {
	return PaginationLink(PaginationLinkProps{Href: href}, g.Text("Siguiente"))
}

func PaginationEllipsis() g.Node {
	return h.Span(
		h.Class("ui-pagination__ellipsis"),
		g.Attr("data-slot", "pagination-ellipsis"),
		g.Attr("aria-hidden", "true"),
		g.Text("…"),
	)
}
