package posts

import (
	"net/http"

	"github.com/3-lines-studio/bifrost/example/vanilla/app/store"
)

func Load(r *http.Request) (any, error) {
	return map[string]any{"posts": store.Posts()}, nil
}

func Cache() string {
	return "public, s-maxage=60"
}
