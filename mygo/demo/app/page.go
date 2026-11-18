package app

import (
	"github.com/3-lines-studio/bifrost/mygo/seo"
	"github.com/egoist/mygo/ui"
)

// View draws the home page. No Load and no props: the same page answers every
// request, so the server renders it once and keeps the HTML.
func View() func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Padding(28).Gap(14).Children(func() {
			ui.Text(c, "One source, three targets").FontSize(34).Bold().Role(ui.RoleHeading).Level(1)
			ui.Text(c, "The server renders this page to HTML, the browser boots the same Go over it, and the desktop app draws it in a window.")
			ui.Row(c).Gap(16).Children(func() {
				seo.Link(c, "Pricing", "/pricing")
				seo.Link(c, "Blog", "/blog")
				seo.Link(c, "About", "/about")
			})
			ui.Text(c, "What the demo shows").FontSize(22).Bold().Role(ui.RoleHeading).Level(2)
			for _, item := range []string{
				"Public pages a crawler indexes, with canonical, social card and JSON-LD.",
				"A blog of pages a database names, listed in the sitemap.",
				"Screens the app draws alone, signed in with a token the server issued.",
				"An error page that keeps the status of what failed.",
			} {
				ui.Text(c, item)
			}
		})
	}
}

// Screen is the page in the app.
func Screen() func(*ui.Context) {
	return View()
}

// Metadata is what a crawler reads: the title, the card a chat shows, the
// words of the subject, the languages the page speaks, and the two things the
// site is.
func Metadata() seo.Head {
	return seo.Head{
		Title:       "Bifrost + MyGo",
		Description: "One Go source for a server-rendered site, a WebAssembly app and a desktop app.",
		Canonical:   "https://demo.bifrost.run/",
		Image:       "https://demo.bifrost.run/card.png",
		Keywords:    "golang, ssr, seo, webassembly, desktop",
		Type:        "website",
		Lang:        "en",
		Alternates: []seo.Alternate{
			{Lang: "en", URL: "https://demo.bifrost.run/"},
			{Lang: "es", URL: "https://demo.bifrost.run/es"},
			{Lang: "x-default", URL: "https://demo.bifrost.run/"},
		},
		Schema: `[
			{"@context":"https://schema.org","@type":"Organization","name":"Bifrost","url":"https://demo.bifrost.run/"},
			{"@context":"https://schema.org","@type":"WebSite","name":"Bifrost + MyGo","url":"https://demo.bifrost.run/"}
		]`,
	}
}

// Cache is the Cache-Control header of the page the server rendered.
func Cache() string {
	return "public, s-maxage=300"
}
