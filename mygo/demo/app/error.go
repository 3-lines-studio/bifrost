package app

import (
	"github.com/3-lines-studio/bifrost/mygo/seo"
	"github.com/egoist/mygo/ui"
)

// Error answers a page whose Load failed, with the status the error wraps or a
// 500. A load that answers 404 shows the not-found page of the app instead, so
// this page never has to draw one.
func Error(error) func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Padding(28).Gap(12).Children(func() {
			ui.Text(c, "Something went wrong").FontSize(32).Bold().Role(ui.RoleHeading).Level(1)
			ui.Text(c, "The page could not load. Try again in a moment.")
			seo.Link(c, "Back to the home page", "/")
		})
	}
}

// ErrorMetadata is what a crawler reads of it.
func ErrorMetadata() seo.Head {
	return seo.Head{Title: "Something went wrong", Robots: "noindex"}
}
