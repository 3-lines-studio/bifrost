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
			ui.Breadcrumb(ui.BreadcrumbList(
				ui.BreadcrumbItem(ui.BreadcrumbLink("#", g.Text("Inicio"))),
				ui.BreadcrumbSeparator(),
				ui.BreadcrumbItem(ui.BreadcrumbPage(g.Text("UI"))),
			)),
			h.P(h.Class("demo-kicker"), g.Text("Alpine UI")),
			h.H1(h.Class("demo-title"), g.Text("Go + Alpine, con contrato de shadcn")),
			h.P(h.Class("demo-lede"), g.Text("Primitives tipados, comportamiento accesible y CSS con tokens.")),
			h.Div(
				h.Class("demo-actions"),
				ui.Button(ui.ButtonProps{Variant: ui.VariantDefault, Size: ui.SizeLG}, g.Text("Empezar")),
				ui.Button(ui.ButtonProps{Variant: ui.VariantOutline}, g.Text("Ver docs")),
				ui.Badge(ui.VariantSecondary, g.Text("v0.1")),
			),
		),
		h.Section(
			h.Class("demo-grid"),
			ui.Card(
				ui.CardHeader(
					ui.CardTitle(g.Text("Button")),
					ui.CardDescription(g.Text("Variantes y tamaños del contrato shadcn.")),
					ui.CardAction(ui.Badge(ui.VariantOutline, g.Text("primitive"))),
				),
				ui.CardContent(
					h.Div(
						h.Class("demo-actions"),
						ui.Button(ui.ButtonProps{}, g.Text("Default")),
						ui.Button(ui.ButtonProps{Variant: ui.VariantSecondary}, g.Text("Secondary")),
						ui.Button(ui.ButtonProps{Variant: ui.VariantDestructive}, g.Text("Destructive")),
						ui.Button(ui.ButtonProps{Variant: ui.VariantGhost}, g.Text("Ghost")),
						ui.Button(ui.ButtonProps{Variant: ui.VariantLink}, g.Text("Link")),
					),
				),
			),
			ui.Card(
				ui.CardHeader(
					ui.CardTitle(g.Text("Field")),
					ui.CardDescription(g.Text("Label, input y ayuda.")),
				),
				ui.CardContent(
					ui.Form(nil,
						ui.FormItem(
							ui.FormLabel("name", g.Text("Nombre")),
							ui.FormControl(ui.Input(ui.InputProps{
								Name:        "name",
								ID:          "name",
								Placeholder: props.Name,
							})),
							ui.FormDescription(g.Text("Se usa en el saludo.")),
						),
					),
				),
			),
		),
		h.Section(
			h.Class("demo-section"),
			h.H2(h.Class("demo-section__title"), g.Text("Tabs")),
			ui.Tabs(ui.TabsProps{Value: "account"},
				ui.TabsList(
					ui.TabsTrigger(ui.TabsTriggerProps{Value: "account"}, g.Text("Cuenta")),
					ui.TabsTrigger(ui.TabsTriggerProps{Value: "billing"}, g.Text("Facturación")),
				),
				ui.TabsContent(ui.TabsContentProps{Value: "account"},
					ui.Card(ui.CardContent(h.P(g.Text("Contenido de la cuenta.")))),
				),
				ui.TabsContent(ui.TabsContentProps{Value: "billing"},
					ui.Card(ui.CardContent(h.P(g.Text("Contenido de facturación.")))),
				),
			),
		),
		h.Section(
			h.Class("demo-section"),
			h.H2(h.Class("demo-section__title"), g.Text("Formularios y datos")),
			ui.Alert(ui.VariantDefault,
				ui.AlertTitle(g.Text("Contrato shadcn")),
				ui.AlertDescription(g.Text("Mismos slots, variantes y tokens; props Go y comportamiento Alpine.")),
			),
			h.Div(
				h.Class("demo-actions"),
				ui.Avatar(ui.AvatarFallback(g.Text("AL"))),
				ui.Label(ui.LabelProps{For: "terms"}, g.Text("Acepto los términos")),
				ui.Checkbox(ui.CheckboxProps{ID: "terms", Name: "terms"}),
				ui.Switch(ui.SwitchProps{ID: "public", Name: "public", Checked: true}),
				ui.Toggle(ui.ToggleProps{Variant: ui.VariantOutline, Pressed: true}, g.Text("Favorito")),
			),
			ui.Progress(ui.ProgressProps{Value: 62}),
			ui.Table(
				ui.TableHeader(ui.TableRow(ui.TableHead(g.Text("Plan")), ui.TableHead(g.Text("Precio")))),
				ui.TableBody(
					ui.TableRow(ui.TableCell(g.Text("Go")), ui.TableCell(g.Text("$0"))),
					ui.TableRow(ui.TableCell(g.Text("Pro")), ui.TableCell(g.Text("$16"))),
				),
			),
			h.Div(
				h.Class("demo-grid"),
				ui.AspectRatio(ui.AspectRatioProps{Ratio: "16 / 9"},
					h.Div(
						g.Attr("style", "display:grid;place-items:center;background:var(--secondary);color:var(--muted-foreground);font-size:0.8rem"),
						g.Text("16 / 9"),
					),
				),
				ui.ScrollArea(
					h.P(g.Text("ScrollArea conserva el scroll nativo y el foco de teclado.")),
					h.P(g.Text("Contenido de ejemplo para demostrar el overflow.")),
					h.P(g.Text("Sin scrollbars custom, sin JavaScript.")),
				),
			),
			ui.Pagination(
				ui.PaginationContent(
					ui.PaginationItem(ui.PaginationPrevious("#")),
					ui.PaginationItem(ui.PaginationLink(ui.PaginationLinkProps{Href: "#", IsActive: true}, g.Text("1"))),
					ui.PaginationItem(ui.PaginationLink(ui.PaginationLinkProps{Href: "#"}, g.Text("2"))),
					ui.PaginationItem(ui.PaginationEllipsis()),
					ui.PaginationItem(ui.PaginationNext("#")),
				),
			),
		),
		h.Section(
			h.Class("demo-section"),
			h.H2(h.Class("demo-section__title"), g.Text("Overlays")),
			ui.Accordion(ui.AccordionProps{Type: "single", DefaultValue: "one", Collapsible: true},
				ui.AccordionItem(ui.AccordionItemProps{Value: "one"},
					ui.AccordionTrigger(ui.AccordionTriggerProps{Value: "one"}, g.Text("¿Qué es el contrato?")),
					ui.AccordionContent(ui.AccordionContentProps{Value: "one"}, g.Text("Nombres, slots, variantes y tokens de shadcn.")),
				),
				ui.AccordionItem(ui.AccordionItemProps{Value: "two"},
					ui.AccordionTrigger(ui.AccordionTriggerProps{Value: "two"}, g.Text("¿Qué cambia en Go?")),
					ui.AccordionContent(ui.AccordionContentProps{Value: "two"}, g.Text("Props tipadas y Alpine en lugar de React.")),
				),
			),
			ui.Collapsible(ui.CollapsibleProps{},
				ui.CollapsibleTrigger(ui.Button(ui.ButtonProps{Variant: ui.VariantGhost}, g.Text("Más detalles"))),
				ui.CollapsibleContent(
					ui.Card(ui.CardContent(h.P(g.Text("Contenido colapsable con Alpine.")))),
				),
			),
			h.Div(
				h.Class("demo-actions"),
				ui.Sheet(
					ui.SheetTrigger(ui.Button(ui.ButtonProps{Variant: ui.VariantOutline}, g.Text("Abrir sheet"))),
					ui.SheetContent(ui.SheetContentProps{Side: "right"},
						ui.SheetHeader(
							ui.SheetTitle(g.Text("Filtros")),
							ui.SheetDescription(g.Text("Panel lateral con el mismo estado de dialog.")),
						),
						ui.SheetFooter(
							ui.SheetClose(ui.Button(ui.ButtonProps{Variant: ui.VariantSecondary}, g.Text("Cerrar"))),
						),
					),
				),
				ui.Popover(
					ui.PopoverTrigger(ui.Button(ui.ButtonProps{Variant: ui.VariantOutline}, g.Text("Abrir popover"))),
					ui.PopoverContent(
						h.P(h.Class("demo-popover-title"), g.Text("Popover")),
						h.P(g.Text("Contenido flotante con cierre por click outside y Escape.")),
					),
				),
				ui.TooltipProvider(
					ui.Tooltip(
						ui.TooltipTrigger(ui.Button(ui.ButtonProps{Variant: ui.VariantGhost, Size: ui.SizeIcon, Attrs: []g.Node{g.Attr("aria-label", "Ayuda")}}, g.Text("?"))),
						ui.TooltipContent(g.Text("Ayuda contextual")),
					),
				),
			),
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
				ui.Dialog(
					ui.DialogTrigger(ui.Button(ui.ButtonProps{Variant: ui.VariantOutline}, g.Text("Abrir dialog"))),
					ui.DialogContent(
						ui.DialogHeader(
							ui.DialogTitle(g.Text("¿Eliminar álbum?")),
							ui.DialogDescription(g.Text("Esta acción no se puede deshacer.")),
						),
						h.P(g.Text("El foco queda atrapado y vuelve al trigger al cerrar.")),
						ui.DialogFooter(
							ui.DialogClose(ui.Button(ui.ButtonProps{Variant: ui.VariantOutline}, g.Text("Cancelar"))),
							ui.Button(ui.ButtonProps{Variant: ui.VariantDestructive}, g.Text("Eliminar")),
						),
					),
				),
				ui.DropdownMenu(
					ui.DropdownMenuTrigger(ui.Button(ui.ButtonProps{Variant: ui.VariantOutline}, g.Text("Acciones"))),
					ui.DropdownMenuContent(
						ui.DropdownMenuLabel(g.Text("Cuenta")),
						ui.DropdownMenuItem(ui.DropdownMenuItemProps{Href: "#"}, g.Text("Editar")),
						ui.DropdownMenuItem(ui.DropdownMenuItemProps{Href: "#"}, g.Text("Compartir")),
						ui.DropdownMenuSeparator(),
						ui.DropdownMenuItem(ui.DropdownMenuItemProps{Href: "#"}, g.Text("Eliminar")),
					),
				),
				ui.Button(ui.ButtonProps{
					Variant: ui.VariantOutline,
					Attrs:   []g.Node{g.Attr("@click", "$store.toasts.push('Guardado', 'success')")},
				}, g.Text("Mostrar toast")),
			),
		),
		ui.ToastRegion(),
	}
}
