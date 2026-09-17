package query

import (
	"net/http"
)

func Load(r *http.Request) (any, error) {
	return map[string]any{
		"pathname": r.URL.EscapedPath(),
		"tags":     r.URL.Query()["tag"],
	}, nil
}
