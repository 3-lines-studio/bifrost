package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type SeparatorProps struct {
	Orientation string
}

func Separator(props SeparatorProps) g.Node {
	orientation := props.Orientation
	if orientation == "" {
		orientation = "horizontal"
	}
	return h.Div(
		h.Class("ui-separator"),
		g.Attr("data-slot", "separator"),
		g.Attr("data-orientation", orientation),
		g.Attr("role", "separator"),
		g.Attr("aria-orientation", orientation),
	)
}
