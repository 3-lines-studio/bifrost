package main

import (
	"log"
	"net/http"
	"os"

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
	addr := os.Getenv("BIFROST_ADDR")
	if addr == "" {
		addr = ":8082"
	}
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Print(err)
	}
}
