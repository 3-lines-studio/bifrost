package slug

import (
	"errors"
	"net/http"

	"github.com/3-lines-studio/bifrost"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type Props struct {
	Name string
}

func Load(r *http.Request) (Props, error) {
	switch r.URL.Query().Get("state") {
	case "error":
		return Props{}, bifrost.Status(http.StatusServiceUnavailable, errors.New("boom"))
	case "missing":
		return Props{}, bifrost.NotFound()
	default:
		return Props{Name: r.PathValue("slug")}, nil
	}
}

func Page(props Props) g.Node {
	return g.Group{
		h.H1(h.Class("text-3xl font-semibold"), g.Text("Hello, "+props.Name)),
		h.A(h.Href("/"), h.Class("mt-4 inline-block text-sky-300"), g.Text("Back")),
	}
}
