package main

import (
	"log"
	"net/http"

	"github.com/3-lines-studio/bifrost"
	"github.com/3-lines-studio/bifrost/example"
)

func main() {
	app := bifrost.New(
		example.BifrostFS,
		bifrost.Page("/", "./pages/home.tsx", bifrost.WithClient()),
		bifrost.Page("/about", "./pages/about.tsx", bifrost.WithClient()),
	)
	defer app.Stop()

	log.Fatal(http.ListenAndServe(":8080", app.Handler()))
}
