package app

import (
	"github.com/3-lines-studio/bifrost/mygo/seo"
	"github.com/egoist/mygo/ui"
)

// Layout wraps every page of the app, on the server and in the window: the
// navigation stays while the pages change under it.
func Layout(page func(*ui.Context)) func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Children(func() {
			ui.Row(c).Padding(20).Gap(18).Children(func() {
				seo.Link(c, "Bifrost + MyGo", "/")
				seo.Link(c, "Pricing", "/pricing")
				seo.Link(c, "Blog", "/blog")
				seo.Link(c, "About", "/about")
				ui.Row(c).Grow(1).Justify(ui.End).Gap(18).Children(func() {
					seo.Link(c, "Dashboard", "/dashboard")
					seo.Link(c, "Sign in", "/login")
				})
			})
			page(c)
		})
	}
}
