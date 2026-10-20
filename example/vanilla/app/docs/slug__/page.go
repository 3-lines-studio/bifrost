package doc

import "net/http"

func Load(r *http.Request) (any, error) {
	return map[string]any{"path": r.PathValue("slug")}, nil
}
