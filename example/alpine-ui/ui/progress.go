package ui

import (
	"fmt"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type ProgressProps struct {
	Value float64
}

func Progress(props ProgressProps) g.Node {
	value := props.Value
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	return h.Div(
		h.Class("ui-progress"),
		g.Attr("data-slot", "progress"),
		g.Attr("role", "progressbar"),
		g.Attr("aria-valuemin", "0"),
		g.Attr("aria-valuemax", "100"),
		g.Attr("aria-valuenow", fmt.Sprintf("%.0f", value)),
		h.Div(
			h.Class("ui-progress__indicator"),
			g.Attr("data-slot", "progress-indicator"),
			g.Attr("style", fmt.Sprintf("width: %.0f%%", value)),
		),
	)
}
