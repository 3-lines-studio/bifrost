package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type AccordionProps struct {
	Type         string
	DefaultValue string
	Collapsible  bool
}

func Accordion(props AccordionProps, children ...g.Node) g.Node {
	kind := props.Type
	if kind == "" {
		kind = "single"
	}
	collapsible := "false"
	if props.Collapsible {
		collapsible = "true"
	}
	id := ID("accordion")
	nodes := []g.Node{
		h.Class("ui-accordion"),
		g.Attr("data-slot", "accordion"),
		g.Attr("x-data", "uiAccordion({id: "+quote(id)+", type: "+quote(kind)+", value: "+quote(props.DefaultValue)+", collapsible: "+collapsible+"})"),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

type AccordionItemProps struct {
	Value string
}

func AccordionItem(props AccordionItemProps, children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-accordion__item"),
		g.Attr("data-slot", "accordion-item"),
		g.Attr("data-value", props.Value),
		g.Attr(":data-state", "isOpen("+quote(props.Value)+") ? 'open' : 'closed'"),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

type AccordionTriggerProps struct {
	Value string
}

func AccordionTrigger(props AccordionTriggerProps, children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-accordion__trigger"),
		g.Attr("data-slot", "accordion-trigger"),
		h.Type("button"),
		g.Attr(":id", "id + '-trigger-"+props.Value+"'"),
		g.Attr(":aria-controls", "id + '-content-"+props.Value+"'"),
		g.Attr(":aria-expanded", "isOpen("+quote(props.Value)+")"),
		g.Attr(":data-state", "isOpen("+quote(props.Value)+") ? 'open' : 'closed'"),
		g.Attr("@click", "toggle("+quote(props.Value)+")"),
	}
	nodes = append(nodes, children...)
	return h.Button(nodes...)
}

type AccordionContentProps struct {
	Value string
}

func AccordionContent(props AccordionContentProps, children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-accordion__content"),
		g.Attr("data-slot", "accordion-content"),
		g.Attr("role", "region"),
		g.Attr(":id", "id + '-content-"+props.Value+"'"),
		g.Attr(":aria-labelledby", "id + '-trigger-"+props.Value+"'"),
		g.Attr("x-show", "isOpen("+quote(props.Value)+")"),
		g.Attr("x-cloak", ""),
		g.Attr(":data-state", "isOpen("+quote(props.Value)+") ? 'open' : 'closed'"),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}
