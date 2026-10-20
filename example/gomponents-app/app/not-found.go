package app

import (
	"net/http"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func NotFound(r *http.Request) g.Node {
	return h.HTML(
		h.Head(h.TitleEl(g.Text("Not found")), h.Link(h.Rel("stylesheet"), h.Href("/styles.css"))),
		h.Body(
			h.Class("page status"),
			h.Main(
				h.Class("status__panel"),
				h.H1(h.Class("status__title"), g.Text("Not found")),
				h.P(h.Class("status__text"), g.Text(r.URL.Path)),
				h.A(h.Href("/"), h.Class("status__link"), g.Text("Back")),
			),
		),
	)
}
