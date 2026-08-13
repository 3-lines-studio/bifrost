package main

import (
	"context"
	"log"
	"net/http"

	"github.com/3-lines-studio/bifrost"
)

type headersPlugin struct{}

func (headersPlugin) Name() string { return "example.headers" }
func (headersPlugin) Register(registry *bifrost.AppRegistry) error {
	if err := registry.AddRoutes(bifrost.Client("/dashboard", "example/plugin/pages/dashboard.tsx")); err != nil {
		return err
	}
	if err := registry.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			w.Header().Set("X-Example-Plugin", "active")
			next.ServeHTTP(w, request)
		})
	}); err != nil {
		return err
	}
	return registry.AssetHeaders(func(header http.Header, _ bool) {
		header.Set("X-Asset-Plugin", "active")
	})
}

func main() {
	app, err := bifrost.New(bifrost.Config{
		Assets:     bifrostAssets,
		AppPlugins: []bifrost.AppPlugin{headersPlugin{}},
		Routes: []bifrost.Route{
			bifrost.Static("/{$}", "example/plugin/pages/home.tsx", nil),
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	if bifrost.Building() {
		return
	}
	defer func() { _ = app.Close(context.Background()) }()
	if err := http.ListenAndServe(":8081", app.Handler()); err != nil {
		log.Print(err)
	}
}
