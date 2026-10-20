package store

type Post struct {
	Slug    string
	Title   string
	Summary string
	Body    string
}

var posts = []Post{
	{Slug: "hello", Title: "Hello, vanilla", Summary: "A page rendered by Go.", Body: "No React, no hydration."},
	{Slug: "turbo", Title: "Turbo navigation", Summary: "Links swap the body and keep the process.", Body: "History, scroll and progress come from Turbo."},
	{Slug: "alpine", Title: "Alpine state", Summary: "Local state without a component runtime.", Body: "Alpine initializes the DOM after every swap."},
}

func Posts() []Post { return posts }

func Find(slug string) (Post, bool) {
	for _, post := range posts {
		if post.Slug == slug {
			return post, true
		}
	}
	return Post{}, false
}
