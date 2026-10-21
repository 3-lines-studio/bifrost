package hello

import (
	"net/http"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func Error(err error, r *http.Request) g.Node {
	return h.HTML(
		h.Head(h.TitleEl(g.Text("Hello error")), h.Link(h.Rel("stylesheet"), h.Href("/styles.css"))),
		h.Body(h.Class("page status"), h.Main(h.Class("status__panel"), h.H1(h.Class("status__title"), g.Text("Hello error")))),
	)
}
