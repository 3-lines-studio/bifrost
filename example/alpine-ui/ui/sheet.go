package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func Sheet(children ...g.Node) g.Node {
	id := ID("sheet")
	nodes := []g.Node{
		h.Class("ui-sheet"),
		g.Attr("data-slot", "sheet"),
		g.Attr("x-data", "uiDialog({id: "+quote(id)+"})"),
		g.Attr(":data-state", "open ? 'open' : 'closed'"),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func SheetTrigger(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-sheet-trigger"),
		g.Attr("data-slot", "sheet-trigger"),
		g.Attr("aria-haspopup", "dialog"),
		g.Attr(":aria-expanded", "open"),
		g.Attr(":aria-controls", "id + '-content'"),
		g.Attr("@click", "show()"),
	}
	nodes = append(nodes, children...)
	return h.Span(nodes...)
}

type SheetContentProps struct {
	Side string
}

func SheetContent(props SheetContentProps, children ...g.Node) g.Node {
	side := props.Side
	if side == "" {
		side = "right"
	}
	nodes := []g.Node{
		h.Class("ui-sheet__panel"),
		g.Attr("data-slot", "sheet-content"),
		g.Attr("data-side", side),
		g.Attr(":id", "id + '-content'"),
		g.Attr(":data-state", "open ? 'open' : 'closed'"),
		g.Attr("x-ref", "dialog"),
		g.Attr(":aria-labelledby", "id + '-title'"),
		g.Attr(":aria-describedby", "id + '-description'"),
		g.Attr("@close", "open = false"),
		g.Attr("@click", "closeOnBackdrop($event)"),
		h.Button(
			h.Type("button"),
			h.Class("ui-sheet__close"),
			g.Attr("data-slot", "sheet-close"),
			g.Attr("@click", "close()"),
			g.Attr("aria-label", "Close sheet"),
			g.Text("×"),
		),
		h.Div(append([]g.Node{h.Class("ui-sheet__inner")}, children...)...),
	}
	return h.Dialog(nodes...)
}

func SheetHeader(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-sheet__header"), g.Attr("data-slot", "sheet-header")}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func SheetFooter(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-sheet__footer"), g.Attr("data-slot", "sheet-footer")}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func SheetTitle(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-sheet__title"),
		g.Attr("data-slot", "sheet-title"),
		g.Attr(":id", "id + '-title'"),
	}
	nodes = append(nodes, children...)
	return h.H2(nodes...)
}

func SheetDescription(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-sheet__description"),
		g.Attr("data-slot", "sheet-description"),
		g.Attr(":id", "id + '-description'"),
	}
	nodes = append(nodes, children...)
	return h.P(nodes...)
}

func SheetClose(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-sheet-close"),
		g.Attr("data-slot", "sheet-close"),
		g.Attr("@click", "close()"),
	}
	nodes = append(nodes, children...)
	return h.Span(nodes...)
}
