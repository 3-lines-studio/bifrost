// Package upload is the screen that sends photos to the server: the app reads
// what a picture says of itself, sends it in parts, and shows the link a phone
// reads to open it. It is a screen of the app, so no crawler reads it.
package upload

import (
	"github.com/3-lines-studio/bifrost/mygo/qr"
	"github.com/3-lines-studio/bifrost/mygo/seo"
	"github.com/egoist/mygo/ui"
)

// Item is one file the screen shows: what it is, how much of it the server has,
// and where it ended.
type Item struct {
	Name string
	Size int64
	Sent int64
	// Note is what the picture says of itself, as "4000×6000 · Canon EOS R5".
	Note string
	// URL is where the server keeps the file, once it has all of it, and Code
	// the same address as a code a phone reads.
	URL  string
	Code *qr.Code
	Err  string
}

// Deps is what the app gives the screen.
type Deps interface {
	Uploads() []Item
	UploadErr() string
	Choose()
}

// Screen draws the uploads.
func Screen(deps Deps) func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Padding(28).Gap(12).Children(func() {
			ui.Text(c, "Upload").FontSize(32).Bold().Role(ui.RoleHeading).Level(1)
			ui.Text(c, "The app reads the file where it is, sends it in parts, and the server joins them.")
			if ui.Button(c, "Choose photos").Clicked() {
				deps.Choose()
			}
			if err := deps.UploadErr(); err != "" {
				ui.Text(c, "The picker failed: "+err)
			}
			items := deps.Uploads()
			if len(items) == 0 {
				ui.Text(c, "No files yet: choose a few, and they go to the server.")
			}
			for _, item := range items {
				ui.Column(c).Gap(6).Padding(12).Children(func() {
					ui.Text(c, item.Name).Bold().Role(ui.RoleHeading).Level(2)
					if item.Size > 0 {
						progress := float64(item.Sent) / float64(item.Size)
						if item.URL != "" {
							progress = 1
						}
						ui.Progress(c, progress).Label("Sent " + item.Name)
					}
					if item.Note != "" {
						ui.Text(c, item.Note).FontSize(13)
					}
					if item.Err != "" {
						ui.Text(c, item.Err)
					}
					if item.URL != "" {
						seo.Link(c, "Open it on the server", item.URL)
						if item.Code != nil {
							qr.Element(c, item.Code, 160)
						}
					}
				})
			}
			seo.Link(c, "Dashboard", "/dashboard")
		})
	}
}

// Metadata is the title of the tab, which is not indexed.
func Metadata() seo.Head {
	return seo.Head{Title: "Upload · Bifrost + MyGo", Robots: "noindex"}
}
