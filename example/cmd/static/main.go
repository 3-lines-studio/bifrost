package main

import (
	"log"
	"net/http"

	"github.com/3-lines-studio/bifrost"
	"github.com/3-lines-studio/bifrost/example"
	"github.com/go-chi/chi/v5"
)

func main() {
	r, err := bifrost.New(bifrost.WithAssetsFS(example.BifrostFS))
	if err != nil {
		log.Fatalf("Failed to start renderer: %v", err)
	}
	defer r.Stop()

	homeHandler := r.NewPage("./pages/about.tsx", bifrost.WithClientOnly())

	router := chi.NewRouter()

	router.Handle("/", homeHandler)

	assetRouter := chi.NewRouter()
	bifrost.RegisterAssetRoutes(assetRouter, r, router)

	addr := ":8080"
	log.Printf("Serving on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, assetRouter); err != nil {
		log.Fatal(err)
	}
}
