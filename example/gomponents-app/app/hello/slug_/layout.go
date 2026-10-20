package slug

import (
	"net/http"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func LayoutContext(content g.Node, r *http.Request) g.Node {
	return h.Section(
		h.Class("rounded-xl border border-slate-800 bg-slate-900 p-6"),
		h.P(h.Class("mb-4 text-xs font-semibold uppercase tracking-widest text-sky-300"), g.Text("Nested layout · "+r.PathValue("slug"))),
		content,
	)
}
