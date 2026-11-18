package main

import (
	"context"
	"log"
	"os"
	"sync"

	"github.com/3-lines-studio/bifrost/mygo/example/api"
	"github.com/3-lines-studio/bifrost/mygo/example/store"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type app struct {
	client api.Client
	win    *mygo.Window

	mu      sync.Mutex
	notes   []store.Note
	loadErr string
}

func main() {
	instance := &app{client: api.Client{BaseURL: apiBase()}}
	mygo.App.WhenReady(func() {
		instance.win = mygo.NewWindow(mygo.WindowOptions{
			Title:   "Bifrost + MyGo",
			Width:   900,
			Height:  700,
			Content: ui.View(instance.view),
		})
		go instance.Refresh()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func apiBase() string {
	if value := os.Getenv("BIFROST_EXAMPLE_API"); value != "" {
		return value
	}
	if value := api.DefaultBaseURL(); value != "" {
		return value
	}
	return "http://localhost:8080"
}

// view draws the page of the path, from the route table bifrost build
// generates out of the Screen functions of the pages.
func (a *app) view(c *ui.Context) {
	router := ui.NewRouter("/")
	router.View(c, func(r *ui.Route) {
		for _, page := range appPages(a) {
			if r.Match(page.Pattern) {
				page.View(c)
				return
			}
		}
		ui.Text(c, "Not found")
	})
}

// Refresh loads the notes again.
func (a *app) Refresh() {
	notes, err := a.client.Notes(context.Background())
	if a.win == nil {
		return
	}
	a.win.Update(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if err != nil {
			a.loadErr = err.Error()
			return
		}
		a.notes, a.loadErr = notes, ""
	})
}

// Notes returns what the last load got.
func (a *app) Notes() []store.Note {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]store.Note(nil), a.notes...)
}

// LoadErr is what the last load failed with.
func (a *app) LoadErr() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.loadErr
}
