package about

import (
	"github.com/3-lines-studio/bifrost/mygo/seo"
	"github.com/egoist/mygo/ui"
)

// View draws the about page, which a crawler reads and the app draws.
func View() func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Padding(28).Gap(14).Children(func() {
			ui.Text(c, "About").FontSize(32).Bold().Role(ui.RoleHeading).Level(1)
			ui.Text(c, "Bifrost serves the page. MyGo draws it. A page.go holds both: the View a search engine reads as HTML, the Screen the app shows, and the Load that feeds them.")
			ui.Text(c, "The server runs the view in a headless MyGo window, reads the tree a screen reader would, and writes the headings, texts and links. The browser boots the same Go as WebAssembly over that HTML. The desktop app draws the same view with a native toolkit.")
			seo.Link(c, "Read the blog", "/blog")
		})
	}
}

// Screen is the page in the app.
func Screen() func(*ui.Context) {
	return View()
}

// Metadata is what a crawler reads.
func Metadata() seo.Head {
	return seo.Head{
		Title:       "About · Bifrost + MyGo",
		Description: "One Go source for the server, the browser and the desktop, with the articles a search engine reads.",
		Canonical:   "https://demo.bifrost.run/about",
		Keywords:    "about, go, ssr, mygo",
	}
}

// Cache keeps the page in a shared cache: it shows the same for everyone.
func Cache() string {
	return "public, s-maxage=3600"
}
