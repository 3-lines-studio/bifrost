package main

import (
	"embed"
	"log"
	"net/http"

	"github.com/3-lines-studio/bifrost"
)

//go:embed all:.bifrost
var BifrostFS embed.FS

func main() {
	app := bifrost.New(
		BifrostFS,
		bifrost.Page("/{$}", "./pages/showcase.svelte"),
	)
	log.Fatal(http.ListenAndServe(":8080", app.Handler()))
}
