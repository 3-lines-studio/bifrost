package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func Table(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-table-container"), g.Attr("data-slot", "table-container")}
	table := []g.Node{h.Class("ui-table"), g.Attr("data-slot", "table")}
	table = append(table, children...)
	nodes = append(nodes, h.Table(table...))
	return h.Div(nodes...)
}

func TableHeader(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-table__header"), g.Attr("data-slot", "table-header")}
	nodes = append(nodes, children...)
	return h.THead(nodes...)
}

func TableBody(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-table__body"), g.Attr("data-slot", "table-body")}
	nodes = append(nodes, children...)
	return h.TBody(nodes...)
}

func TableFooter(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-table__footer"), g.Attr("data-slot", "table-footer")}
	nodes = append(nodes, children...)
	return h.TFoot(nodes...)
}

func TableRow(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-table__row"), g.Attr("data-slot", "table-row")}
	nodes = append(nodes, children...)
	return h.Tr(nodes...)
}

func TableHead(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-table__head"), g.Attr("data-slot", "table-head")}
	nodes = append(nodes, children...)
	return h.Th(nodes...)
}

func TableCell(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-table__cell"), g.Attr("data-slot", "table-cell")}
	nodes = append(nodes, children...)
	return h.Td(nodes...)
}

func TableCaption(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-table__caption"), g.Attr("data-slot", "table-caption")}
	nodes = append(nodes, children...)
	return h.Caption(nodes...)
}
