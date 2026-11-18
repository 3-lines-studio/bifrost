// Package post is the page of one post of the blog: the folder name slug_ makes
// the segment a parameter, and Load reads it from the request.
package post

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/3-lines-studio/bifrost"
	"github.com/3-lines-studio/bifrost/mygo/demo/store"
	"github.com/3-lines-studio/bifrost/mygo/seo"
	"github.com/egoist/mygo/ui"
)

// Props is what the loader hands the view and the metadata.
type Props struct {
	Post store.Post
}

// Boom is the slug that fails on purpose, so the demo shows the error page and
// the test of the demo reads it.
const Boom = "boom"

// Load reads the post of the slug the request names, and answers 404 when no
// post has it.
func Load(r *http.Request) (any, error) {
	if r.PathValue("slug") == Boom {
		return nil, errors.New("the store of posts is down")
	}
	post, ok := store.FindPost(r.PathValue("slug"))
	if !ok {
		return nil, bifrost.NotFound()
	}
	return Props{Post: post}, nil
}

// View draws the post.
func View(props Props) func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Gap(10).Children(func() {
			ui.Text(c, props.Post.Date).FontSize(13)
			ui.Text(c, props.Post.Title).FontSize(32).Bold().Role(ui.RoleHeading).Level(1)
			ui.Text(c, props.Post.Summary)
			seo.Link(c, "Back to the blog", "/blog")
		})
	}
}

// Metadata is what a crawler reads of the post.
func Metadata(props Props) seo.Head {
	canonical := "https://demo.bifrost.run/blog/" + props.Post.Slug
	return seo.Head{
		Title:       props.Post.Title + " · Bifrost + MyGo",
		Description: props.Post.Summary,
		Canonical:   canonical,
		Image:       "https://demo.bifrost.run/card.png",
		Type:        "article",
		Keywords:    "go, ssr, " + props.Post.Slug,
		Schema: fmt.Sprintf(
			`{"@context":"https://schema.org","@type":"BlogPosting","headline":%q,"datePublished":%q,"mainEntityOfPage":%q}`,
			props.Post.Title, props.Post.Date, canonical,
		),
	}
}

// Deps is what the app gives the screen: the path the router shows, which is
// the only place a screen reads the parameter of its page.
type Deps interface {
	Path() string
}

// Screen is the page in the app, at the path the router shows.
func Screen(deps Deps) func(*ui.Context) {
	slug := strings.TrimPrefix(deps.Path(), "/blog/")
	post, ok := store.FindPost(slug)
	if !ok {
		return func(c *ui.Context) {
			ui.Text(c, "No post at this path.").FontSize(24).Bold().Role(ui.RoleHeading).Level(1)
		}
	}
	return View(Props{Post: post})
}

// Sitemap lists the pages of this route: the pattern has a parameter, so only
// the loader knows the paths a crawler reads.
func Sitemap() []string {
	posts := store.Posts()
	paths := make([]string, 0, len(posts))
	for _, post := range posts {
		paths = append(paths, "/blog/"+post.Slug)
	}
	return paths
}
