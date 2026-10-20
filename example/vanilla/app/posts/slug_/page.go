package post

import (
	"errors"
	"net/http"

	"github.com/3-lines-studio/bifrost"
	"github.com/3-lines-studio/bifrost/example/vanilla/app/store"
)

func Load(r *http.Request) (any, error) {
	slug := r.PathValue("slug")
	if slug == "boom" {
		return nil, bifrost.Status(http.StatusServiceUnavailable, errors.New("intentional loader error"))
	}
	post, ok := store.Find(slug)
	if !ok {
		return nil, bifrost.NotFound()
	}
	return map[string]any{"post": post}, nil
}
