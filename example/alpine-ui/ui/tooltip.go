package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func TooltipProvider(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-tooltip-provider"), g.Attr("data-slot", "tooltip-provider")}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func Tooltip(children ...g.Node) g.Node {
	id := ID("tooltip")
	nodes := []g.Node{
		h.Class("ui-tooltip"),
		g.Attr("data-slot", "tooltip"),
		g.Attr("x-data", "uiTooltip({id: "+quote(id)+"})"),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func TooltipTrigger(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-tooltip__trigger"),
		g.Attr("data-slot", "tooltip-trigger"),
		g.Attr(":aria-describedby", "open ? id : null"),
		g.Attr("@mouseenter", "show()"),
		g.Attr("@mouseleave", "hide()"),
		g.Attr("@focusin", "show()"),
		g.Attr("@focusout", "hide()"),
		g.Attr("@keydown.escape", "hide()"),
	}
	nodes = append(nodes, children...)
	return h.Span(nodes...)
}

func TooltipContent(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-tooltip__content"),
		g.Attr("data-slot", "tooltip-content"),
		g.Attr("role", "tooltip"),
		g.Attr(":id", "id"),
		g.Attr("x-show", "open"),
		g.Attr("x-cloak", ""),
		g.Attr(":data-state", "open ? 'open' : 'closed'"),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}
