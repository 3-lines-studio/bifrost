package app

import (
	"github.com/3-lines-studio/bifrost/mygo/seo"
	"github.com/egoist/mygo/ui"
)

// View draws the home page: the server renders it to HTML for crawlers, and
// the app draws the same view.
func View() func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Padding(24).Gap(12).Children(func() {
			ui.Text(c, "Bifrost + MyGo").FontSize(32).Bold().Role(ui.RoleHeading).Level(1)
			ui.Text(c, "One source, three targets: the server, the browser and the desktop.")
			seo.Link(c, "Notes", "/notes")
		})
	}
}

// Metadata is what a crawler reads.
func Metadata() seo.Head {
	return seo.Head{
		Title:       "Bifrost + MyGo",
		Description: "Bifrost native pages rendered by MyGo",
		Canonical:   "https://example.com/",
	}
}

// Screen is the page in the app.
func Screen() func(*ui.Context) {
	return View()
}

// Cache is the Cache-Control header of the server-rendered page.
func Cache() string {
	return "public, s-maxage=60"
}
