// Package dashboard is the screen of the notes: the app draws it from what the
// API answered, and no crawler reads it.
package dashboard

import (
	"github.com/3-lines-studio/bifrost/mygo/demo/store"
	"github.com/3-lines-studio/bifrost/mygo/seo"
	"github.com/egoist/mygo/ui"
)

// Deps is what the app gives the screen: the notes it loaded, the profile the
// server answered with, and the calls that load them again.
type Deps interface {
	Notes() []store.Note
	LoadErr() string
	Refresh()
	Profile() string
	LoadProfile()
	SignOut()
	SignedIn() bool
}

// Screen draws the notes.
func Screen(deps Deps) func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Padding(28).Gap(12).Children(func() {
			ui.Text(c, "Dashboard").FontSize(32).Bold().Role(ui.RoleHeading).Level(1)
			ui.Text(c, "This screen is the app's: the server answers a shell nobody indexes.")
			ui.Row(c).Gap(12).Children(func() {
				if ui.Button(c, "Load again").Clicked() {
					deps.Refresh()
				}
				if deps.SignedIn() {
					if ui.Button(c, "Who am I").Clicked() {
						deps.LoadProfile()
					}
					if ui.Button(c, "Sign out").Clicked() {
						deps.SignOut()
					}
				}
			})
			if profile := deps.Profile(); profile != "" {
				ui.Text(c, "The server reads this as "+profile+".")
			}
			if err := deps.LoadErr(); err != "" {
				ui.Text(c, "The API failed: "+err)
			}
			notes := deps.Notes()
			if notes == nil {
				ui.Text(c, "Loading…")
			}
			for _, note := range notes {
				ui.Text(c, note.Title)
			}
			if !deps.SignedIn() {
				seo.Link(c, "Sign in to reach the API", "/login")
			}
		})
	}
}

// Metadata is the title of the tab, which is not indexed.
func Metadata() seo.Head {
	return seo.Head{Title: "Dashboard · Bifrost + MyGo", Robots: "noindex"}
}
