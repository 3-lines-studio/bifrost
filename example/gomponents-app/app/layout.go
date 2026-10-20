package app

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func Layout(content g.Node) g.Node {
	return h.HTML(
		g.Attr("lang", "en"),
		h.Head(
			h.Meta(h.Charset("utf-8")),
			h.Meta(h.Name("viewport"), h.Content("width=device-width, initial-scale=1")),
			h.TitleEl(g.Text("Gomponents App Router")),
			h.Link(h.Rel("stylesheet"), h.Href("/styles.css")),
			h.Script(h.Defer(), h.Src("https://cdn.jsdelivr.net/npm/@hotwired/turbo@8/dist/turbo.es2017-umd.js")),
			h.Script(h.Defer(), h.Src("https://cdn.jsdelivr.net/npm/alpinejs@3/dist/cdn.min.js")),
			h.StyleEl(g.Raw("[x-cloak]{display:none!important}")),
		),
		h.Body(
			h.Class("min-h-screen bg-slate-950 text-slate-100"),
			h.Main(h.Class("mx-auto max-w-4xl space-y-10 px-6 py-12"), content),
		),
	)
}
