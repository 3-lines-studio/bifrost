package blog

import (
	"net/http"

	"github.com/3-lines-studio/bifrost/mygo/demo/store"
	"github.com/3-lines-studio/bifrost/mygo/seo"
	"github.com/egoist/mygo/ui"
)

// Props is what the loader hands the view.
type Props struct {
	Posts []store.Post
}

// Load reads the published posts.
func Load(*http.Request) (any, error) {
	return Props{Posts: store.Posts()}, nil
}

// View draws the list of posts.
func View(props Props) func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Gap(12).Children(func() {
			ui.Text(c, "Blog").FontSize(32).Bold().Role(ui.RoleHeading).Level(1)
			for _, post := range props.Posts {
				ui.Column(c).Gap(4).Children(func() {
					seo.Link(c, post.Title, "/blog/"+post.Slug)
					ui.Text(c, post.Date+" · "+post.Summary)
				})
			}
		})
	}
}

// Metadata is what a crawler reads of the list.
func Metadata() seo.Head {
	return seo.Head{
		Title:       "Blog · Bifrost + MyGo",
		Description: "Notes on rendering one Go source to a server, a browser and a desktop app.",
		Canonical:   "https://demo.bifrost.run/blog",
		Keywords:    "blog, go, ssr",
	}
}

// Screen is the page in the app.
func Screen() func(*ui.Context) {
	return View(Props{Posts: store.Posts()})
}

// Cache keeps the list in a shared cache for a minute.
func Cache() string {
	return "public, s-maxage=60"
}
