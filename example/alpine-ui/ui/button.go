package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

const (
	VariantDefault     = "default"
	VariantDestructive = "destructive"
	VariantOutline     = "outline"
	VariantSecondary   = "secondary"
	VariantGhost       = "ghost"
	VariantLink        = "link"

	SizeDefault = "default"
	SizeSM      = "sm"
	SizeLG      = "lg"
	SizeIcon    = "icon"
)

type ButtonProps struct {
	Variant  string
	Size     string
	Type     string
	Disabled bool
	Attrs    []g.Node
}

func Button(props ButtonProps, children ...g.Node) g.Node {
	return buttonNode(false, "", props, children...)
}

type LinkButtonProps struct {
	Href     string
	Variant  string
	Size     string
	Disabled bool
	Attrs    []g.Node
}

func LinkButton(props LinkButtonProps, children ...g.Node) g.Node {
	return buttonNode(true, props.Href, ButtonProps{
		Variant:  props.Variant,
		Size:     props.Size,
		Disabled: props.Disabled,
		Attrs:    props.Attrs,
	}, children...)
}

func buttonNode(link bool, href string, props ButtonProps, children ...g.Node) g.Node {
	variant := props.Variant
	if variant == "" {
		variant = VariantDefault
	}
	size := props.Size
	if size == "" {
		size = SizeDefault
	}
	nodes := []g.Node{
		h.Class("ui-button"),
		g.Attr("data-slot", "button"),
		g.Attr("data-variant", variant),
		g.Attr("data-size", size),
	}
	if link {
		nodes = append(nodes, h.Href(href))
	} else {
		kind := props.Type
		if kind == "" {
			kind = "button"
		}
		nodes = append(nodes, h.Type(kind))
	}
	nodes = append(nodes, props.Attrs...)
	if props.Disabled {
		nodes = append(nodes, h.Disabled())
		if link {
			nodes = append(nodes, g.Attr("aria-disabled", "true"))
		}
	}
	nodes = append(nodes, children...)
	if link {
		return h.A(nodes...)
	}
	return h.Button(nodes...)
}
