package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"

	"github.com/3-lines-studio/bifrost"
)

func main() {
	app, err := bifrost.New(bifrost.Config{
		Routes: []bifrost.Route{
			bifrost.Native("/{$}", renderHome, nil).WithCache("public, s-maxage=60"),
			bifrost.Native("/post/{slug}", renderPost, loadPost),
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = app.Close(context.Background()) }()
	if err := bifrost.Serve(context.Background(), app.Handler()); err != nil {
		log.Print(err)
	}
}

func renderHome(context.Context, json.RawMessage) (bifrost.NativePage, error) {
	return bifrost.NativePage{
		Title:       "Bifrost native pages",
		Description: "A page rendered by Go, with no frontend build",
		Canonical:   "https://example.com/",
		Body:        []byte(`<main><h1>Native pages</h1><p><a href="/post/hello">A post</a></p></main>`),
	}, nil
}

func loadPost(request *http.Request) (any, error) {
	slug := request.PathValue("slug")
	if slug == "" {
		return nil, bifrost.NotFound()
	}
	return bifrost.PageData{
		Props:    map[string]string{"slug": slug},
		Document: bifrost.Document{Lang: "en"},
	}, nil
}

func renderPost(_ context.Context, props json.RawMessage) (bifrost.NativePage, error) {
	var data struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(props, &data); err != nil {
		return bifrost.NativePage{}, err
	}
	return bifrost.NativePage{
		Title: fmt.Sprintf("Post %s", data.Slug),
		Body:  []byte(fmt.Sprintf(`<main><h1>%s</h1></main>`, html.EscapeString(data.Slug))),
	}, nil
}
