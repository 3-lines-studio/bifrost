package app

import (
	"github.com/3-lines-studio/bifrost/mygo/seo"
	"github.com/egoist/mygo/ui"
)

// NotFound answers the paths no page holds, with a 404.
func NotFound() func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Padding(28).Gap(12).Children(func() {
			ui.Text(c, "We could not find that page").FontSize(32).Bold().Role(ui.RoleHeading).Level(1)
			ui.Text(c, "The path is not a page of the site, and not a screen of the app.")
			seo.Link(c, "Back to the home page", "/")
		})
	}
}

// NotFoundMetadata is what a crawler reads of it.
func NotFoundMetadata() seo.Head {
	return seo.Head{Title: "Not found · Bifrost + MyGo", Robots: "noindex"}
}
