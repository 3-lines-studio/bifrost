// Package seo renders a MyGo native UI view to HTML for crawlers.
//
// It runs the view in MyGo's headless tester, reads the accessibility tree of
// the first frame, and writes the parts that carry content: headings, texts
// and links. Use it for public pages that search engines index, and serve the
// result through bifrost.Native.
package seo

import (
	"html"
	"strconv"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

const (
	defaultWidth  = 390
	defaultHeight = 844
	maxHeading    = 6
)

// Options configures a snapshot. Width and Height are the viewport the view
// lays out in, in DIPs; zero uses a mobile viewport, as crawlers read.
// Locale is the language the view shows, as "es-AR", which its messages and
// its text take; empty is the one MyGo draws without. Direction lays the view
// out from the right when it is ui.RightToLeft. Links maps an accessible name
// to the URL it opens, for the links a view draws with ui.Link: Link records
// its own, and a recorded one wins. A link with neither renders as its text.
type Options struct {
	Width     int
	Height    int
	Locale    string
	Direction ui.Direction
	Links     map[string]string
}

// Body renders view and returns the HTML of its content inside a main element,
// or nil when the view shows no heading, text, or mapped link.
//
// The output is flat: the tree tells reading order, not layout. Controls,
// images, and lists render nothing beyond the texts they hold, and a text
// whose Label overrides its text renders the label, as screen readers read it.
// Calls are serialized.
func Body(view func(*ui.Context), options Options) []byte {
	if view == nil {
		return nil
	}
	width, height := options.Width, options.Height
	if width <= 0 {
		width = defaultWidth
	}
	if height <= 0 {
		height = defaultHeight
	}

	renderMu.Lock()
	recorded := map[string]string{}
	recording = recorded
	tester := ui.NewTester(view, width, height)
	if options.Locale != "" {
		tester.SetLocale(options.Locale)
	}
	if options.Direction == ui.RightToLeft {
		tester.SetDirection(ui.RightToLeft)
	}
	elements := tester.AccessAll()
	recording = nil
	renderMu.Unlock()
	links := mergeLinks(options.Links, recorded)

	var content strings.Builder
	var heading ui.Rect
	inHeading := false
	for _, element := range elements {
		if inHeading && inside(element.Bounds, heading) {
			continue
		}
		inHeading = false
		if !writeElement(&content, element, links) {
			continue
		}
		if element.Role == ui.RoleHeading {
			heading, inHeading = element.Bounds, true
		}
	}
	if content.Len() == 0 {
		return nil
	}
	return []byte("<main>" + content.String() + "</main>")
}

var (
	renderMu  sync.Mutex
	recording map[string]string
)

// record keeps the URL of a link drawn while a view renders for the server.
// renderMu holds one render at a time, so the map belongs to that render.
func record(text, url string) {
	if recording == nil {
		return
	}
	name := strings.TrimSpace(text)
	if name == "" || url == "" {
		return
	}
	recording[name] = url
}

// mergeLinks lets a link the view recorded win over the map it was given.
func mergeLinks(given, recorded map[string]string) map[string]string {
	if len(given) == 0 && len(recorded) == 0 {
		return nil
	}
	links := make(map[string]string, len(given)+len(recorded))
	for name, url := range given {
		links[name] = url
	}
	for name, url := range recorded {
		links[name] = url
	}
	return links
}

func writeElement(out *strings.Builder, element ui.AccessInfo, links map[string]string) bool {
	name := strings.TrimSpace(element.Name)
	if name == "" {
		return false
	}
	switch element.Role {
	case ui.RoleHeading:
		level := element.Level
		if level < 1 || level > maxHeading {
			level = 2
		}
		tag := "h" + strconv.Itoa(level)
		out.WriteString("<" + tag + ">")
		out.WriteString(html.EscapeString(name))
		out.WriteString("</" + tag + ">")
	case ui.RoleText:
		writeText(out, name)
	case ui.RoleLink:
		href := strings.TrimSpace(links[name])
		if href == "" {
			writeText(out, name)
			return true
		}
		out.WriteString(`<p><a href="`)
		out.WriteString(html.EscapeString(href))
		out.WriteString(`">`)
		out.WriteString(html.EscapeString(name))
		out.WriteString(`</a></p>`)
	default:
		return false
	}
	return true
}

func writeText(out *strings.Builder, text string) {
	out.WriteString("<p>")
	out.WriteString(html.EscapeString(text))
	out.WriteString("</p>")
}

func inside(inner, outer ui.Rect) bool {
	if inner.W == 0 || inner.H == 0 {
		return false
	}
	return inner.X >= outer.X && inner.Y >= outer.Y &&
		inner.X+inner.W <= outer.X+outer.W && inner.Y+inner.H <= outer.Y+outer.H
}
