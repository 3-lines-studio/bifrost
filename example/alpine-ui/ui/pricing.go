package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type Plan struct {
	Name        string
	Price       string
	Period      string
	Description string
	Features    []string
	CTA         string
	Href        string
	Highlight   bool
}

type PricingProps struct {
	Title       string
	Description string
	Plans       []Plan
}

func Pricing(props PricingProps) g.Node {
	plans := make([]g.Node, 0, len(props.Plans))
	for _, plan := range props.Plans {
		variant := Outline
		if plan.Highlight {
			variant = Primary
		}
		features := make([]g.Node, 0, len(plan.Features))
		for _, feature := range plan.Features {
			features = append(features, h.Li(h.Class("ui-pricing__feature"), g.Text(feature)))
		}
		plans = append(plans, h.Article(
			h.Class("ui-pricing__plan"),
			g.Attr("data-highlight", boolString(plan.Highlight)),
			h.Header(
				h.Class("ui-pricing__header"),
				h.H3(h.Class("ui-pricing__name"), g.Text(plan.Name)),
				h.P(h.Class("ui-pricing__description"), g.Text(plan.Description)),
			),
			h.P(
				h.Class("ui-pricing__price"),
				g.Text(plan.Price),
				h.Span(h.Class("ui-pricing__period"), g.Text(plan.Period)),
			),
			h.Ul(h.Class("ui-pricing__features"), g.Group(features)),
			LinkButton(LinkButtonProps{Href: plan.Href, Variant: variant}, g.Text(plan.CTA)),
		))
	}
	return h.Section(
		h.Class("ui-pricing"),
		h.Header(
			h.Class("ui-pricing__intro"),
			h.H2(h.Class("ui-pricing__title"), g.Text(props.Title)),
			h.P(h.Class("ui-pricing__lede"), g.Text(props.Description)),
		),
		h.Div(h.Class("ui-pricing__grid"), g.Group(plans)),
	)
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
