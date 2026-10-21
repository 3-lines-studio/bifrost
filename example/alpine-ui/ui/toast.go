package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func ToastRegion() g.Node {
	return h.Div(
		h.Class("ui-toasts"),
		g.Attr("x-data", ""),
		g.Attr("aria-live", "polite"),
		g.Attr("aria-atomic", "true"),
		h.Template(
			g.Attr("x-for", "toast in $store.toasts.items"),
			g.Attr(":key", "toast.id"),
			h.Div(
				h.Class("ui-toast"),
				g.Attr(":data-type", "toast.type"),
				h.P(h.Class("ui-toast__message"), g.Attr("x-text", "toast.message")),
				h.Button(
					h.Type("button"),
					h.Class("ui-toast__close"),
					g.Attr("@click", "$store.toasts.remove(toast.id)"),
					g.Attr("aria-label", "Dismiss notification"),
					g.Text("×"),
				),
			),
		),
	)
}
