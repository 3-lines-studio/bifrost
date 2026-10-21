package slug

import (
	"net/http"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func LayoutContext(content g.Node, r *http.Request) g.Node {
	return h.Section(
		h.Class("nested"),
		h.P(h.Class("nested__label"), g.Text("Nested layout · "+r.PathValue("slug"))),
		content,
	)
}
