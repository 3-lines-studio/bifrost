package hello

import (
	"net/http"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func Error(err error, r *http.Request) g.Node {
	return h.HTML(
		h.Head(h.TitleEl(g.Text("Hello error"))),
		h.Body(h.Class("bg-slate-950 p-12 text-slate-100"), h.H1(g.Text("Hello error"))),
	)
}
