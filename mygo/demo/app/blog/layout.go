package blog

import (
	"github.com/3-lines-studio/bifrost/mygo/seo"
	"github.com/egoist/mygo/ui"
)

// Layout wraps the pages of the blog, inside the layout of the app: the pages
// of the blog change under it.
func Layout(page func(*ui.Context)) func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Padding(28).Gap(10).Children(func() {
			seo.Link(c, "← Blog", "/blog")
			page(c)
		})
	}
}
