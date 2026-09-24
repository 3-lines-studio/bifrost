package demo

import (
	"net/http"

	store "github.com/3-lines-studio/bifrost/example/app-router-demo/app/store"
)

func Cache() string {
	return "public, s-maxage=60"
}

func Load(r *http.Request) (any, error) {
	return map[string]any{
		"projects": len(store.Projects()),
		"docs":     len(store.DocPaths()),
		"loads":    store.Loads("/"),
	}, nil
}
