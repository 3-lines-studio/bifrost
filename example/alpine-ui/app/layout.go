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
			h.TitleEl(g.Text("Alpine UI registry")),
			h.Link(h.Rel("stylesheet"), h.Href("/ui/ui.css")),
			h.Link(h.Rel("stylesheet"), h.Href("/app.css")),
			h.Script(h.Defer(), h.Src("/ui/ui.js")),
			h.Script(h.Defer(), h.Src("https://cdn.jsdelivr.net/npm/alpinejs@3/dist/cdn.min.js")),
		),
		h.Body(
			h.Class("demo-page"),
			g.Attr("x-data", ""),
			h.Main(h.Class("demo-shell"), content),
		),
	)
}
