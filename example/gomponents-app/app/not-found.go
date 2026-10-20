package app

import (
	"net/http"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func NotFound(r *http.Request) g.Node {
	return h.HTML(
		h.Head(h.TitleEl(g.Text("Not found"))),
		h.Body(
			h.Class("min-h-screen bg-slate-950 p-12 text-slate-100"),
			h.H1(h.Class("text-3xl font-semibold"), g.Text("Not found")),
			h.P(h.Class("mt-2 text-slate-400"), g.Text(r.URL.Path)),
			h.A(h.Href("/"), h.Class("mt-4 inline-block text-sky-300"), g.Text("Back")),
		),
	)
}
