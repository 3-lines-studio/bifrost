package ui

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func Avatar(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-avatar"), g.Attr("data-slot", "avatar")}
	nodes = append(nodes, children...)
	return h.Span(nodes...)
}

func AvatarImage(src, alt string) g.Node {
	return h.Img(
		h.Class("ui-avatar__image"),
		g.Attr("data-slot", "avatar-image"),
		h.Src(src),
		h.Alt(alt),
	)
}

func AvatarFallback(children ...g.Node) g.Node {
	nodes := []g.Node{h.Class("ui-avatar__fallback"), g.Attr("data-slot", "avatar-fallback")}
	nodes = append(nodes, children...)
	return h.Span(nodes...)
}
