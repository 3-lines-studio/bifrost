package seo

import "github.com/egoist/mygo/ui"

// Head is what Metadata of an App Router Go page returns: the document a
// search engine reads before it reads the page. The zero value says nothing,
// and the generated server leaves those fields out.
//
//	func Metadata(props Props) seo.Head {
//		return seo.Head{
//			Title:       props.Title,
//			Description: "Pricing for teams of every size",
//			Canonical:   "https://example.com/pricing",
//			Image:       "https://example.com/card.png",
//		}
//	}
type Head struct {
	Title       string
	Description string
	Canonical   string
	Image       string
	Robots      string
	Schema      string
	Keywords    string
	Type        string
	Lang        string
	Dir         string
	Alternates  []Alternate
}

// Alternate is the same page in another language: the server writes it as a
// link element a crawler follows. Lang is a language tag as "es" or "es-AR",
// or "x-default" for the page served when no language matches.
type Alternate struct {
	Lang string
	URL  string
}

// Link draws a link and tells the server where it goes: the accessibility
// tree MyGo lays out carries a link's name but not its URL, and a crawler
// follows the URL.
//
// It draws the same link ui.Link draws, in the window and in the browser, so
// a page uses it everywhere. A view that calls ui.Link instead leaves the
// server guessing, and needs Options.Links.
func Link(c *ui.Context, text, url string) ui.Element {
	record(text, url)
	return ui.Link(c, text, url)
}
