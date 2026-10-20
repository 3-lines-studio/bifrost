package app

import (
	"fmt"
	"net/http"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type User struct {
	Name string
	Bio  string
}

type CardProps struct {
	Title   string
	Summary string
	Kicker  string
	Tags    []string
}

type MenuItem struct {
	Label string
	Href  string
}

type Props struct {
	User  User
	Cards []CardProps
	Menu  []MenuItem
}

func Load(r *http.Request) (Props, error) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Ada Lovelace"
	}
	items := make([]CardProps, 6)
	for index := range items {
		items[index] = CardProps{
			Title: fmt.Sprintf("Card %02d", index+1),
			Tags:  []string{"go", "gomponents"},
		}
	}
	return Props{
		User:  User{Name: name, Bio: "Mathematician and first programmer."},
		Cards: items,
		Menu: []MenuItem{
			{Label: "Home", Href: "/"},
			{Label: "Hello", Href: "/hello/world"},
		},
	}, nil
}

func Page(props Props) g.Node {
	return g.Group{
		h.Header(
			h.Class("hero"),
			h.P(h.Class("hero__kicker"), g.Text("Go page")),
			h.H1(h.Class("hero__title"), g.Text("Gomponents + Alpine + vanilla CSS")),
			h.P(h.Class("hero__lede"), g.Text("One page.go with the loader, the page, and its components.")),
		),
		h.Div(
			h.Class("toolbar"),
			Dropdown("Open menu", props.Menu),
			Counter(),
		),
		UserCard(props.User),
		h.Section(
			h.Class("cards"),
			g.Map(props.Cards, func(card CardProps) g.Node {
				return Card(card)
			}),
		),
	}
}

func UserCard(user User) g.Node {
	return h.Section(
		h.Class("section"),
		h.H2(h.Class("section__title"), g.Text("Parent passes variables")),
		Card(CardProps{
			Title:   user.Name,
			Summary: user.Bio,
			Kicker:  "Profile",
			Tags:    []string{"go", "gomponents", "alpine"},
		}),
	)
}

func Card(props CardProps) g.Node {
	summary := props.Summary
	if summary == "" {
		summary = "Default summary"
	}
	kicker := props.Kicker
	if kicker == "" {
		kicker = "Component"
	}
	return h.Article(
		h.Class("card"),
		h.P(h.Class("card__kicker"), g.Text(kicker)),
		h.H2(h.Class("card__title"), g.Text(props.Title)),
		h.P(h.Class("card__summary"), g.Text(summary)),
		g.If(len(props.Tags) > 0,
			h.Ul(h.Class("card__tags"),
				g.Map(props.Tags, func(tag string) g.Node {
					return h.Li(h.Class("tag"), g.Text(tag))
				}),
			),
		),
	)
}

func Dropdown(label string, items []MenuItem) g.Node {
	return h.Div(
		g.Attr("x-data", "{ open: false }"),
		g.Attr("@click.outside", "open = false"),
		h.Class("dropdown"),
		h.Button(
			h.Type("button"),
			g.Attr("@click", "open = !open"),
			g.Attr(":aria-expanded", "open"),
			h.Class("dropdown__button"),
			g.Text(label),
		),
		h.Div(
			g.Attr("x-show", "open"),
			g.Attr("x-cloak", ""),
			h.Class("dropdown__menu"),
			g.Map(items, func(item MenuItem) g.Node {
				return h.A(
					h.Href(item.Href),
					h.Class("dropdown__item"),
					g.Text(item.Label),
				)
			}),
		),
	)
}

func Counter() g.Node {
	return h.Div(
		g.Attr("x-data", "{ count: 0 }"),
		h.Class("counter"),
		h.Button(
			h.Type("button"),
			g.Attr("@click", "count++"),
			h.Class("counter__button"),
			g.Text("Increment"),
		),
		h.Output(
			g.Attr("x-text", "count"),
			h.Class("counter__value"),
			g.Text("0"),
		),
	)
}
