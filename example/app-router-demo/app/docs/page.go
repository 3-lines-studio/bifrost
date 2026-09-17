package docs

import (
	"net/http"

	store "github.com/3-lines-studio/bifrost/example/app-router-demo/app/store"
)

func Load(r *http.Request) (any, error) {
	return map[string]any{
		"paths": store.DocPaths(),
		"loads": store.Loads("/docs"),
	}, nil
}
