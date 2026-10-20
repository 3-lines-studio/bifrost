package slug

import (
	"net/http"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type Props struct {
	Name string
}

func Load(r *http.Request) (Props, error) {
	return Props{Name: r.PathValue("slug")}, nil
}

func Page(props Props) g.Node {
	return h.HTML(
		h.Head(
			h.Meta(h.Charset("utf-8")),
			h.Meta(h.Name("viewport"), h.Content("width=device-width, initial-scale=1")),
			h.TitleEl(g.Text("Hello "+props.Name)),
		),
		h.Body(
			h.Class("min-h-screen bg-slate-950 p-12 text-slate-100"),
			h.H1(h.Class("text-3xl font-semibold"), g.Text("Hello, "+props.Name)),
			h.A(h.Href("/"), h.Class("mt-4 inline-block text-sky-300"), g.Text("Back")),
		),
	)
}
