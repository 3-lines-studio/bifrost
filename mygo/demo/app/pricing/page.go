package pricing

import (
	"net/http"

	"github.com/3-lines-studio/bifrost/mygo/demo/store"
	"github.com/3-lines-studio/bifrost/mygo/seo"
	"github.com/egoist/mygo/ui"
)

// Props is what the loader hands the view and the metadata.
type Props struct {
	Plans []store.Plan
}

// Load reads the plans. It runs on the server only: the window and the browser
// get the props of the render.
func Load(*http.Request) (any, error) {
	return Props{Plans: store.Plans()}, nil
}

// View draws the page.
func View(props Props) func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Padding(28).Gap(14).Children(func() {
			ui.Text(c, "Pricing").FontSize(32).Bold().Role(ui.RoleHeading).Level(1)
			ui.Text(c, "Every plan runs the same source on your server, your browser and your desktop.")
			for _, plan := range props.Plans {
				ui.Column(c).Gap(6).Children(func() {
					ui.Text(c, plan.Name+" · "+plan.Price+" USD a month").FontSize(20).Bold().Role(ui.RoleHeading).Level(2)
					ui.Text(c, plan.Includes)
				})
			}
			seo.Link(c, "Read the blog", "/blog")
		})
	}
}

// Metadata reads the props the loader returned.
func Metadata(props Props) seo.Head {
	names := ""
	for index, plan := range props.Plans {
		if index > 0 {
			names += ", "
		}
		names += plan.Name
	}
	return seo.Head{
		Title:       "Pricing · Bifrost + MyGo",
		Description: "Plans for teams of every size: " + names + ".",
		Canonical:   "https://demo.bifrost.run/pricing",
		Keywords:    "pricing, plans",
		Schema:      `{"@context":"https://schema.org","@type":"Product","name":"Bifrost","description":"One Go source for the server, the browser and the desktop"}`,
	}
}

// Screen is the page in the app. It draws the plans the server loaded.
func Screen() func(*ui.Context) {
	return View(Props{Plans: store.Plans()})
}

// Cache keeps the page in a shared cache: the plans are the same for everyone.
func Cache() string {
	return "public, s-maxage=600"
}
