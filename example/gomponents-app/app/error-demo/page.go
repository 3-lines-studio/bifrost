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
	return h.P(h.Class("status__text"), g.Text(props.Message))
}

func Layout(content g.Node, props Props) g.Node {
	return h.HTML(
		h.Head(h.TitleEl(g.Text("Error demo")), h.Link(h.Rel("stylesheet"), h.Href("/styles.css"))),
		h.Body(h.Class("page status"), h.Main(h.Class("status__panel"), content)),
	)
}

func Error(err error, r *http.Request) g.Node {
	return h.HTML(
		h.Head(h.TitleEl(g.Text("Custom error")), h.Link(h.Rel("stylesheet"), h.Href("/styles.css"))),
		h.Body(
			h.Class("page status"),
			h.Main(
				h.Class("status__panel"),
				h.H1(h.Class("status__title"), g.Text("Custom error")),
				h.P(h.Class("status__text"), g.Text(err.Error())),
			),
		),
	)
}

func NotFound(r *http.Request) g.Node {
	return h.HTML(
		h.Head(h.TitleEl(g.Text("Custom not found")), h.Link(h.Rel("stylesheet"), h.Href("/styles.css"))),
		h.Body(
			h.Class("page status"),
			h.Main(
				h.Class("status__panel"),
				h.H1(h.Class("status__title"), g.Text("Custom not found")),
				h.P(h.Class("status__text"), g.Text(r.URL.Path)),
			),
		),
	)
}
