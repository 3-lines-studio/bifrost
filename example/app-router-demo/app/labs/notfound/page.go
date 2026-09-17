package notfound

import (
	"net/http"

	"github.com/3-lines-studio/bifrost"
)

func Load(r *http.Request) (any, error) {
	if r.URL.Query().Get("ok") == "1" {
		return map[string]any{"ok": true}, nil
	}
	return nil, bifrost.NotFound()
}
