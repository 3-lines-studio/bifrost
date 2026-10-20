package app

import (
	"net/http"

	"github.com/3-lines-studio/bifrost/example/vanilla/app/store"
)

func Load(r *http.Request) (any, error) {
	return map[string]any{"posts": len(store.Posts())}, nil
}
