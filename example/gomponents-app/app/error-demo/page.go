package errordemo

import (
	"errors"
	"net/http"

	"github.com/3-lines-studio/bifrost"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type Props struct {
	Message string
}

func Load(r *http.Request) (Props, error) {
	switch r.URL.Query().Get("state") {
	case "error":
		return Props{}, bifrost.Status(http.StatusServiceUnavailable, errors.New("boom"))
	case "missing":
		return Props{}, bifrost.NotFound()
	default:
		return Props{Message: "Everything is fine"}, nil
	}
}

func Cache() string {
	return "public, max-age=60"
}

func Page(props Props) g.Node {
	return h.P(h.Class("text-slate-300"), g.Text(props.Message))
}

func Layout(content g.Node, props Props) g.Node {
	return h.HTML(
		h.Head(h.TitleEl(g.Text("Error demo"))),
		h.Body(h.Class("bg-slate-950 p-12 text-slate-100"), h.Main(content)),
	)
}

func Error(err error, r *http.Request) g.Node {
	return h.HTML(
		h.Head(h.TitleEl(g.Text("Custom error"))),
		h.Body(
			h.Class("bg-slate-950 p-12 text-slate-100"),
			h.H1(g.Text("Custom error")),
			h.P(h.Class("text-slate-400"), g.Text(err.Error())),
		),
	)
}

func NotFound(r *http.Request) g.Node {
	return h.HTML(
		h.Head(h.TitleEl(g.Text("Custom not found"))),
		h.Body(
			h.Class("bg-slate-950 p-12 text-slate-100"),
			h.H1(g.Text("Custom not found")),
			h.P(h.Class("text-slate-400"), g.Text(r.URL.Path)),
		),
	)
}
