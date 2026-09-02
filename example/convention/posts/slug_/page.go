package slug

import (
	"errors"
	"net/http"

	"github.com/3-lines-studio/bifrost"
	app "github.com/3-lines-studio/bifrost/example/convention"
)

func Load(r *http.Request) (any, error) {
	switch r.URL.Query().Get("result") {
	case "redirect":
		return nil, bifrost.Redirect("/")
	case "not-found":
		return nil, bifrost.NotFound()
	case "forbidden":
		return nil, bifrost.Status(http.StatusForbidden, errors.New("access denied"))
	case "error":
		return nil, errors.New("loader failed")
	}
	order, _ := r.Context().Value(app.MiddlewareOrder).([]string)
	return map[string]any{
		"slug":       r.PathValue("slug"),
		"middleware": order,
	}, nil
}
