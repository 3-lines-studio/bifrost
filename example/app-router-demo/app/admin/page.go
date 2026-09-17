package admin

import (
	"net/http"

	app "github.com/3-lines-studio/bifrost/example/app-router-demo/app"
)

func Load(r *http.Request) (any, error) {
	cookie, err := r.Cookie(Cookie)
	return map[string]any{
		"middleware": app.Order(r),
		"cookie":     err == nil && cookie.Value == "ok",
		"pathname":   r.URL.Path,
	}, nil
}
