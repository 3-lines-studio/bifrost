package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type MenuItem struct {
	Label string
	Href  string
}

type MenuProps struct {
	Label string
	Align string
	Items []MenuItem
}

func Menu(props MenuProps) g.Node {
	align := props.Align
	if align == "" {
		align = "start"
	}
	id := ID("menu")
	items := make([]g.Node, 0, len(props.Items))
	for _, item := range props.Items {
		items = append(items, h.A(
			h.Href(item.Href),
			h.Class("ui-menu__item"),
			g.Attr("role", "menuitem"),
			g.Attr("tabindex", "-1"),
			g.Attr("@click", "close(false)"),
			g.Text(item.Label),
		))
	}
	return h.Div(
		h.Class("ui-menu"),
		g.Attr("x-data", "uiMenu()"),
		g.Attr("@click.outside", "close(false)"),
		g.Attr("@keydown.escape.stop", "close()"),
		h.Button(
			h.Type("button"),
			h.Class("ui-button"),
			g.Attr("data-variant", "outline"),
			g.Attr("data-size", "md"),
			g.Attr("aria-haspopup", "menu"),
			g.Attr("aria-controls", id),
			g.Attr(":aria-expanded", "open"),
			g.Attr("x-ref", "button"),
			g.Attr("@click", "toggle()"),
			g.Attr("@keydown.down.prevent", "openAndFocus()"),
			g.Text(props.Label),
		),
		h.Div(
			h.Class("ui-menu__panel"),
			h.ID(id),
			g.Attr("role", "menu"),
			g.Attr("x-ref", "menu"),
			g.Attr("x-show", "open"),
			g.Attr("x-cloak", ""),
			g.Attr(":data-state", "open ? 'open' : 'closed'"),
			g.Attr("data-align", align),
			g.Attr("@keydown", "onKeydown($event)"),
			g.Group(items),
		),
	)
}
