package hash

import (
	"net/http"

	"github.com/3-lines-studio/bifrost/example/app-router-demo/app/store"
)

func Load(r *http.Request) (any, error) {
	return map[string]any{"loads": store.Loads("/labs/hash")}, nil
}
