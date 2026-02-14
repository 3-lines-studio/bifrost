package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/3-lines-studio/bifrost"
	"github.com/3-lines-studio/bifrost/example"
	"github.com/go-chi/chi/v5"
	webview "github.com/webview/webview_go"
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

	server := &http.Server{
		Handler: assetRouter,
		Addr:    "127.0.0.1:0",
	}

	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		log.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	localURL := fmt.Sprintf("http://%s", listener.Addr().String())

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	debug := os.Getenv("BIFROST_DEV") == "1"

	w := webview.New(debug)
	defer w.Destroy()

	w.SetTitle("Bifrost Desktop")
	w.SetSize(1024, 768, webview.HintNone)
	w.Navigate(localURL)

	w.Run()

	if err := server.Shutdown(context.Background()); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
}
