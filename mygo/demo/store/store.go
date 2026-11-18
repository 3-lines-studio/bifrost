// Package store holds what the demo serves: the posts a crawler reads, and the
// notes the app draws and the API changes. It stands in for a database.
package store

import (
	"strings"
	"sync"
)

// Post is a published page: the content the blog serves, and the slugs the
// sitemap lists.
type Post struct {
	Slug    string
	Title   string
	Summary string
	Date    string
}

var posts = []Post{
	{
		Slug:    "one-source-three-targets",
		Title:   "One source, three targets",
		Summary: "The server, the browser and the desktop read the page.go of the same page.",
		Date:    "2026-01-12",
	},
	{
		Slug:    "seo-without-a-browser",
		Title:   "SEO without a browser",
		Summary: "MyGo lays the page out in memory, and Bifrost writes what a crawler reads.",
		Date:    "2026-02-03",
	},
	{
		Slug:    "screens-are-not-pages",
		Title:   "Screens are not pages",
		Summary: "A page has a View a search engine reads; a screen draws alone, in the app.",
		Date:    "2026-02-27",
	},
}

// Posts returns every published post.
func Posts() []Post {
	return append([]Post(nil), posts...)
}

// FindPost returns the post of a slug, and whether it is published.
func FindPost(slug string) (Post, bool) {
	for _, post := range posts {
		if post.Slug == slug {
			return post, true
		}
	}
	return Post{}, false
}

// Plan is a price a reader compares.
type Plan struct {
	Name     string `json:"name"`
	Price    string `json:"price"`
	Includes string `json:"includes"`
}

var plans = []Plan{
	{Name: "Solo", Price: "0", Includes: "One app, the server and the browser build."},
	{Name: "Team", Price: "19", Includes: "Ten apps, a sitemap, and the desktop builds."},
	{Name: "Studio", Price: "49", Includes: "Everything, and a hand on the wiring."},
}

// Plans returns what the pricing page shows.
func Plans() []Plan {
	return append([]Plan(nil), plans...)
}

// Note is what the app lists and the API changes.
type Note struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

var (
	mu     sync.Mutex
	notes  = []Note{{ID: 1, Title: "The server renders the page"}, {ID: 2, Title: "The app draws the same view"}}
	nextID = 3
)

// Notes returns the notes, oldest first.
func Notes() []Note {
	mu.Lock()
	defer mu.Unlock()
	return append([]Note(nil), notes...)
}

// AddNote keeps a note and returns it.
func AddNote(title string) Note {
	mu.Lock()
	defer mu.Unlock()
	note := Note{ID: nextID, Title: strings.TrimSpace(title)}
	nextID++
	notes = append(notes, note)
	return note
}
