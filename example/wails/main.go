package main

import (
	"context"
	"log"

	"github.com/3-lines-studio/bifrost"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	web, err := bifrost.New(bifrost.Config{
		Assets:     bifrostAssets,
		SourceRoot: "frontend",
		Routes: []bifrost.Route{
			bifrost.Client("/{$}", "pages/app.tsx"),
			bifrost.Client("/{path...}", "pages/app.tsx"),
		},
	})
	if err != nil {
		return err
	}
	if bifrost.Building() {
		return nil
	}
	defer func() { _ = web.Close(context.Background()) }()

	native := application.New(application.Options{
		Name:        "Bifrost Wails",
		Description: "Bifrost client routes in a Wails application",
		Services: []application.Service{
			application.NewService(&GreetService{}),
		},
		Assets: application.AssetOptions{
			Handler: web.Handler(),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	native.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Bifrost Wails",
		Width:            960,
		Height:           640,
		MinWidth:         360,
		MinHeight:        480,
		BackgroundColour: application.NewRGB(10, 15, 28),
		URL:              "/",
	})
	return native.Run()
}
