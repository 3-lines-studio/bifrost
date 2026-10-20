package hello

import (
	"net/http"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func NotFound(r *http.Request) g.Node {
	return h.HTML(
		h.Head(h.TitleEl(g.Text("Hello not found")), h.Link(h.Rel("stylesheet"), h.Href("/styles.css"))),
		h.Body(h.Class("page status"), h.Main(h.Class("status__panel"), h.H1(h.Class("status__title"), g.Text("Hello not found")))),
	)
}
