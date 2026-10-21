package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type TabsProps struct {
	Value string
	Attrs []g.Node
}

func Tabs(props TabsProps, children ...g.Node) g.Node {
	id := ID("tabs")
	value := props.Value
	if value == "" {
		value = "tab-1"
	}
	nodes := []g.Node{
		h.Class("ui-tabs"),
		g.Attr("data-slot", "tabs"),
		g.Attr("x-data", "uiTabs({id: "+quote(id)+", value: "+quote(value)+"})"),
	}
	nodes = append(nodes, props.Attrs...)
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func TabsList(children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("ui-tabs__list"),
		g.Attr("data-slot", "tabs-list"),
		g.Attr("role", "tablist"),
		g.Attr("x-ref", "list"),
		g.Attr("@keydown", "onKeydown($event)"),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

type TabsTriggerProps struct {
	Value    string
	Disabled bool
}

func TabsTrigger(props TabsTriggerProps, children ...g.Node) g.Node {
	value := props.Value
	selector := "id + '-tab-" + value + "'"
	panel := "id + '-panel-" + value + "'"
	nodes := []g.Node{
		h.Class("ui-tabs__trigger"),
		g.Attr("data-slot", "tabs-trigger"),
		g.Attr("type", "button"),
		g.Attr("role", "tab"),
		g.Attr(":id", selector),
		g.Attr(":aria-controls", panel),
		g.Attr(":aria-selected", "value === "+quote(value)),
		g.Attr(":tabindex", "value === "+quote(value)+" ? 0 : -1"),
		g.Attr("@click", "select("+quote(value)+")"),
	}
	if props.Disabled {
		nodes = append(nodes, h.Disabled())
	}
	nodes = append(nodes, children...)
	return h.Button(nodes...)
}

type TabsContentProps struct {
	Value string
	Attrs []g.Node
}

func TabsContent(props TabsContentProps, children ...g.Node) g.Node {
	value := props.Value
	panel := "id + '-panel-" + value + "'"
	selector := "id + '-tab-" + value + "'"
	nodes := []g.Node{
		h.Class("ui-tabs__content"),
		g.Attr("data-slot", "tabs-content"),
		g.Attr("role", "tabpanel"),
		g.Attr(":id", panel),
		g.Attr(":aria-labelledby", selector),
		g.Attr("x-show", "value === "+quote(value)),
		g.Attr("x-cloak", ""),
	}
	nodes = append(nodes, props.Attrs...)
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}
