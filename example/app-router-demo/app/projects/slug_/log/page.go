package log

import (
	"net/http"

	"github.com/3-lines-studio/bifrost"
	store "github.com/3-lines-studio/bifrost/example/app-router-demo/app/store"
)

func Load(r *http.Request) (any, error) {
	project, ok := store.FindProject(r.PathValue("slug"))
	if !ok {
		return nil, bifrost.NotFound()
	}
	return map[string]any{"project": project}, nil
}
