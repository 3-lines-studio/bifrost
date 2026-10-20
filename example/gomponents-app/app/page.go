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
	return h.HTML(
		g.Attr("lang", "en"),
		h.Head(
			h.Meta(h.Charset("utf-8")),
			h.Meta(h.Name("viewport"), h.Content("width=device-width, initial-scale=1")),
			h.TitleEl(g.Text("Gomponents App Router")),
			h.Script(h.Src("https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4")),
			h.Script(h.Defer(), h.Src("https://cdn.jsdelivr.net/npm/alpinejs@3/dist/cdn.min.js")),
			h.StyleEl(g.Raw("[x-cloak]{display:none!important}")),
		),
		h.Body(
			h.Class("min-h-screen bg-slate-950 text-slate-100"),
			h.Main(
				h.Class("mx-auto max-w-4xl space-y-10 px-6 py-12"),
				h.Header(
					h.Class("space-y-3"),
					h.P(h.Class("text-xs font-semibold uppercase tracking-[0.3em] text-sky-300"), g.Text("Go page")),
					h.H1(h.Class("text-3xl font-semibold tracking-tight"), g.Text("Gomponents + Alpine + Tailwind")),
					h.P(h.Class("max-w-2xl text-slate-400"), g.Text("One page.go with the loader, the page, and its components.")),
				),
				h.Div(
					h.Class("flex flex-wrap items-center gap-4"),
					Dropdown("Open menu", props.Menu),
					Counter(),
				),
				UserCard(props.User),
				h.Section(
					h.Class("grid gap-4 sm:grid-cols-2"),
					g.Map(props.Cards, func(card CardProps) g.Node {
						return Card(card)
					}),
				),
			),
		),
	)
}

func UserCard(user User) g.Node {
	return h.Section(
		h.Class("space-y-4"),
		h.H2(h.Class("text-sm font-semibold uppercase tracking-widest text-slate-500"), g.Text("Parent passes variables")),
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
		h.Class("rounded-xl border border-slate-800 bg-slate-900 p-5"),
		h.P(h.Class("mb-2 text-xs font-semibold uppercase tracking-widest text-sky-300"), g.Text(kicker)),
		h.H2(h.Class("text-lg font-semibold text-slate-100"), g.Text(props.Title)),
		h.P(h.Class("mt-1 text-sm text-slate-400"), g.Text(summary)),
		g.If(len(props.Tags) > 0,
			h.Ul(h.Class("mt-4 flex flex-wrap gap-2"),
				g.Map(props.Tags, func(tag string) g.Node {
					return h.Li(h.Class("rounded-full border border-slate-700 px-2 py-0.5 text-xs text-slate-400"), g.Text(tag))
				}),
			),
		),
	)
}

func Dropdown(label string, items []MenuItem) g.Node {
	return h.Div(
		g.Attr("x-data", "{ open: false }"),
		g.Attr("@click.outside", "open = false"),
		h.Class("relative inline-block"),
		h.Button(
			h.Type("button"),
			g.Attr("@click", "open = !open"),
			g.Attr(":aria-expanded", "open"),
			h.Class("rounded-lg border border-slate-700 bg-slate-900 px-4 py-2 text-sm text-slate-100"),
			g.Text(label),
		),
		h.Div(
			g.Attr("x-show", "open"),
			g.Attr("x-cloak", ""),
			h.Class("absolute left-0 z-10 mt-2 w-48 rounded-lg border border-slate-700 bg-slate-900 p-1 shadow-xl"),
			g.Map(items, func(item MenuItem) g.Node {
				return h.A(
					h.Href(item.Href),
					h.Class("block rounded-md px-3 py-2 text-sm text-slate-300 hover:bg-slate-800"),
					g.Text(item.Label),
				)
			}),
		),
	)
}

func Counter() g.Node {
	return h.Div(
		g.Attr("x-data", "{ count: 0 }"),
		h.Class("flex items-center gap-3"),
		h.Button(
			h.Type("button"),
			g.Attr("@click", "count++"),
			h.Class("rounded-lg bg-sky-400 px-4 py-2 text-sm font-semibold text-slate-950"),
			g.Text("Increment"),
		),
		h.Output(
			g.Attr("x-text", "count"),
			h.Class("text-sm text-slate-400"),
			g.Text("0"),
		),
	)
}
