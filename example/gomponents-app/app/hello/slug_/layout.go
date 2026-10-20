package slug

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func Layout(content g.Node) g.Node {
	return h.Section(
		h.Class("rounded-xl border border-slate-800 bg-slate-900 p-6"),
		h.P(h.Class("mb-4 text-xs font-semibold uppercase tracking-widest text-sky-300"), g.Text("Nested layout")),
		content,
	)
}
