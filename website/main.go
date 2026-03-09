package main

import (
	"context"
	"embed"
	"log"
	"net/http"
	"os"

	"github.com/3-lines-studio/bifrost"
	"github.com/3-lines-studio/bifrost/website/internal/content"
)

//go:embed all:.bifrost all:public
var bifrostFS embed.FS

func main() {
	loader := content.NewLoader("content")

	routes := []bifrost.Route{
		bifrost.Page("/{$}", "./pages/home.tsx",
			bifrost.WithStaticData(func(ctx context.Context) ([]bifrost.StaticPathData, error) {
				pages, err := loader.LoadAll()
				if err != nil {
					return nil, err
				}
				nav := content.BuildNav(pages)
				return []bifrost.StaticPathData{
					{
						Path: "/",
						Props: map[string]any{
							"nav": nav,
						},
					},
				}, nil
			}),
		),
		bifrost.Page("/docs/{slug...}", "./pages/docs.tsx",
			bifrost.WithStaticData(func(ctx context.Context) ([]bifrost.StaticPathData, error) {
				pages, err := loader.LoadAll()
				if err != nil {
					return nil, err
				}
				nav := content.BuildNav(pages)
				paths := make([]bifrost.StaticPathData, len(pages))
				for i, p := range pages {
					paths[i] = bifrost.StaticPathData{
						Path: "/docs/" + p.Slug,
						Props: map[string]any{
							"title":       p.Title,
							"description": p.Description,
							"html":        p.HTML,
							"slug":        p.Slug,
							"nav":         nav,
						},
					}
				}
				return paths, nil
			}),
		),
	}

	app := bifrost.New(bifrostFS, routes...)
	defer app.Stop()

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("Bifrost website running on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, app.Handler()))
}
