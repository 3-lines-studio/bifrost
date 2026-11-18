// Package notes is the notes screen: the app draws it, and no crawler
// reads it, so it has no View.
package notes

import (
	"github.com/3-lines-studio/bifrost/mygo/example/store"
	"github.com/3-lines-studio/bifrost/mygo/seo"
	"github.com/egoist/mygo/ui"
)

// Deps is what the app gives the screen: the notes it loaded and how to load
// them again. The generated route table passes *app, and the compiler checks
// it has these methods.
type Deps interface {
	Notes() []store.Note
	LoadErr() string
	Refresh()
}

// Screen draws the notes.
func Screen(deps Deps) func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Padding(24).Gap(12).Children(func() {
			ui.Text(c, "Notes").FontSize(28).Bold().Role(ui.RoleHeading).Level(1)
			switch {
			case deps.LoadErr() != "":
				ui.Text(c, deps.LoadErr())
			case deps.Notes() == nil:
				ui.Text(c, "Loading…")
			}
			for _, note := range deps.Notes() {
				ui.Text(c, note.Title)
			}
			if ui.Button(c, "Refresh").Clicked() {
				deps.Refresh()
			}
		})
	}
}

// Metadata is the title of the tab, which is not indexed.
func Metadata() seo.Head {
	return seo.Head{Title: "Notes"}
}
