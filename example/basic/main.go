package main

import (
	"context"
	"log"
	"net/http"

	"github.com/3-lines-studio/bifrost"
)

func main() {
	app, err := bifrost.New(bifrost.Config{
		Assets:            bifrostAssets,
		RenderConcurrency: 2,
		Routes: []bifrost.Route{
			bifrost.Server("/{$}", "example/basic/pages/home.tsx", func(r *http.Request) (any, error) {
				return bifrost.PageData{
					Props:    map[string]string{"name": r.URL.Query().Get("name")},
					Document: bifrost.Document{Lang: "es", Class: "theme-dark", Dir: "ltr"},
				}, nil
			}),
			bifrost.Server("/stream", "example/basic/pages/stream.tsx", nil),
			bifrost.Static("/about", "example/basic/pages/about.tsx", nil),
			bifrost.Static("/post/{slug}", "example/basic/pages/post.tsx", func(context.Context) ([]bifrost.StaticPage, error) {
				return []bifrost.StaticPage{
					{Path: "/post/first", Props: map[string]string{"title": "First"}, Document: bifrost.Document{Lang: "pt-BR", Dir: "ltr"}},
					{Path: "/post/second", Props: map[string]string{"title": "Second"}},
				}, nil
			}),
			bifrost.Client("/app", "example/basic/pages/app.tsx"),
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	if bifrost.Building() {
		return
	}
	defer func() { _ = app.Close(context.Background()) }()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		if err := app.Ready(r.Context()); err != nil {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := app.Register(mux); err != nil {
		log.Fatal(err)
	}
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Print(err)
	}
}
