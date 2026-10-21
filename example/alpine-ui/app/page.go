package app

import (
	"net/http"

	"github.com/3-lines-studio/bifrost/example/alpine-ui/ui"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type Props struct {
	Name string
}

func Load(r *http.Request) (Props, error) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Ada"
	}
	return Props{Name: name}, nil
}

func Page(props Props) g.Node {
	return g.Group{
		h.Header(
			h.Class("demo-hero"),
			h.P(h.Class("demo-kicker"), g.Text("Alpine UI")),
			h.H1(h.Class("demo-title"), g.Text("Go + Alpine, con contrato de shadcn")),
			h.P(h.Class("demo-lede"), g.Text("Primitives tipados, comportamiento accesible y CSS con tokens.")),
			h.Div(
				h.Class("demo-actions"),
				ui.Button(ui.ButtonProps{Variant: ui.Primary, Size: ui.Large}, g.Text("Empezar")),
				ui.Button(ui.ButtonProps{Variant: ui.Outline}, g.Text("Ver docs")),
			),
		),
		h.Section(
			h.Class("demo-grid"),
			ui.Card(ui.CardProps{
				Title:       "Button",
				Description: "Variantes y tamaños por atributos.",
				Body: g.Group{
					ui.Button(ui.ButtonProps{}, g.Text("Default")),
					g.Text(" "),
					ui.Button(ui.ButtonProps{Variant: ui.Primary}, g.Text("Primary")),
					g.Text(" "),
					ui.Button(ui.ButtonProps{Variant: ui.Danger}, g.Text("Danger")),
					g.Text(" "),
					ui.Button(ui.ButtonProps{Variant: ui.Ghost}, g.Text("Ghost")),
				},
			}),
			ui.Card(ui.CardProps{
				Title:       "Field",
				Description: "Label, input y ayuda.",
				Body: ui.Field(ui.FieldProps{
					Label: "Nombre",
					ID:    "name",
					Hint:  "Se usa en el saludo.",
					Control: ui.Input(ui.InputProps{
						Name:        "name",
						ID:          "name",
						Placeholder: props.Name,
					}),
				}),
			}),
		),
		ui.Pricing(ui.PricingProps{
			Title:       "Un block Pines-style",
			Description: "Compuesto solo con primitives y tokens.",
			Plans: []ui.Plan{
				{Name: "Go", Price: "$0", Period: "/mes", Description: "Para empezar.", Features: []string{"Páginas Go", "Alpine", "CSS"}, CTA: "Empezar", Href: "#"},
				{Name: "Pro", Price: "$16", Period: "/mes", Description: "Para vender.", Features: []string{"Todo lo de Go", "Admin", "Soporte"}, CTA: "Elegir Pro", Href: "#", Highlight: true},
			},
		}),
		h.Section(
			h.Class("demo-section"),
			h.H2(h.Class("demo-section__title"), g.Text("Behavior")),
			h.Div(
				h.Class("demo-actions"),
				ui.Dialog(ui.DialogProps{
					ID:           "demo-dialog",
					Title:        "¿Eliminar álbum?",
					Description:  "Esta acción no se puede deshacer.",
					TriggerLabel: "Abrir dialog",
					Body:         h.P(g.Text("El foco queda atrapado y vuelve al trigger al cerrar.")),
					Footer: g.Group{
						ui.Button(ui.ButtonProps{Variant: ui.Outline, Attrs: []g.Node{g.Attr("@click", "close()")}}, g.Text("Cancelar")),
						ui.Button(ui.ButtonProps{Variant: ui.Danger}, g.Text("Eliminar")),
					},
				}),
				ui.Menu(ui.MenuProps{
					Label: "Acciones",
					Items: []ui.MenuItem{
						{Label: "Editar", Href: "#"},
						{Label: "Compartir", Href: "#"},
						{Label: "Eliminar", Href: "#"},
					},
				}),
				ui.Button(ui.ButtonProps{
					Variant: ui.Outline,
					Attrs:   []g.Node{g.Attr("@click", "$store.toasts.push('Guardado', 'success')")},
				}, g.Text("Mostrar toast")),
			),
		),
		ui.ToastRegion(),
	}
}
