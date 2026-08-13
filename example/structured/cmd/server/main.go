package main

import (
	"log"
	"net/http"

	"github.com/3-lines-studio/bifrost"
	"github.com/3-lines-studio/bifrost/example/structured/internal/webapp"
)

func main() {
	app, err := webapp.New(bifrostAssets)
	if err != nil {
		log.Fatal(err)
	}
	if bifrost.Building() {
		return
	}
	defer webapp.Close(app)
	handler, err := webapp.Handler(app)
	if err != nil {
		log.Fatal(err)
	}
	if err := http.ListenAndServe(":8082", handler); err != nil {
		log.Print(err)
	}
}
