package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"

	"github.com/3-lines-studio/bifrost"
	"github.com/3-lines-studio/bifrost/example"
	"github.com/getlantern/systray"
	webview "github.com/webview/webview_go"
)

func main() {
	app := bifrost.New(
		example.BifrostFS,
		bifrost.Page("/", "./pages/about.tsx", bifrost.WithClient()),
	)
	defer app.Stop()

	server := &http.Server{
		Handler: app.Handler(),
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

	var quitOnce sync.Once
	quit := make(chan struct{})

	systray.Run(func() {
		systray.SetTitle("Bifrost")
		systray.SetTooltip("Bifrost Desktop App")
		systray.SetIcon(example.IconPNG)

		mQuit := systray.AddMenuItem("Quit", "Quit Bifrost")

		go func() {
			<-mQuit.ClickedCh
			quitOnce.Do(func() {
				w.Terminate()
				close(quit)
			})
		}()
	}, func() {})

	go func() {
		w.Run()
		quitOnce.Do(func() {
			systray.Quit()
			close(quit)
		})
	}()

	<-quit

	if err := server.Shutdown(context.Background()); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
}
