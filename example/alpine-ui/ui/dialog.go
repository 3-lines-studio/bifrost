package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func Dialog(children ...g.Node) g.Node {
	id := ID("dialog")
	nodes := []g.Node{
		h.Class("ui-dialog"),
		g.Attr("data-slot", "dialog"),
		g.Attr("x-data", "uiDialog({id: "+quote(id)+"})"),
		g.Attr(":data-state", "open ? 'open' : 'closed'"),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func DialogTrigger(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-dialog-trigger"),
		g.Attr("data-slot", "dialog-trigger"),
		g.Attr("aria-haspopup", "dialog"),
		g.Attr(":aria-expanded", "open"),
		g.Attr(":aria-controls", "id + '-content'"),
		g.Attr("@click", "show()"),
	}
	nodes = append(nodes, children...)
	return h.Span(nodes...)
}

func DialogContent(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-dialog__panel"),
		g.Attr("data-slot", "dialog-content"),
		g.Attr(":id", "id + '-content'"),
		g.Attr(":data-state", "open ? 'open' : 'closed'"),
		g.Attr("x-ref", "dialog"),
		g.Attr(":aria-labelledby", "id + '-title'"),
		g.Attr(":aria-describedby", "id + '-description'"),
		g.Attr("@close", "open = false"),
		g.Attr("@click", "closeOnBackdrop($event)"),
		h.Button(
			h.Type("button"),
			h.Class("ui-dialog__close"),
			g.Attr("data-slot", "dialog-close"),
			g.Attr("@click", "close()"),
			g.Attr("aria-label", "Close dialog"),
			g.Text("×"),
		),
		h.Div(append([]g.Node{h.Class("ui-dialog__inner")}, children...)...),
	}
	return h.Dialog(nodes...)
}

func DialogHeader(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-dialog__header"), g.Attr("data-slot", "dialog-header")}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func DialogFooter(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-dialog__footer"), g.Attr("data-slot", "dialog-footer")}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func DialogTitle(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-dialog__title"),
		g.Attr("data-slot", "dialog-title"),
		g.Attr(":id", "id + '-title'"),
	}
	nodes = append(nodes, children...)
	return h.H2(nodes...)
}

func DialogDescription(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-dialog__description"),
		g.Attr("data-slot", "dialog-description"),
		g.Attr(":id", "id + '-description'"),
	}
	nodes = append(nodes, children...)
	return h.P(nodes...)
}

func DialogClose(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-dialog-close"),
		g.Attr("data-slot", "dialog-close"),
		g.Attr("@click", "close()"),
	}
	nodes = append(nodes, children...)
	return h.Span(nodes...)
}
