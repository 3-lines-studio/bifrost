package seo

import (
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func contentView(c *ui.Context) {
	state := &ui.ListState{}
	ui.Column(c).Children(func() {
		ui.Text(c, "Bifrost").FontSize(32).Role(ui.RoleHeading).Level(1)
		ui.Text(c, "Pages rendered by Go.")
		ui.Link(c, "Docs", "https://example.com/docs")
		ui.PrimaryButton(c, "Start")
		ui.List(c, state, 2, func(index int) {
			ui.Text(c, "Row "+strconv.Itoa(index))
		})
	})
}

func TestBodyContent(t *testing.T) {
	got := string(Body(contentView, Options{Links: map[string]string{"Docs": "https://example.com/docs"}}))
	want := `<main><h1>Bifrost</h1><p>Pages rendered by Go.</p><p><a href="https://example.com/docs">Docs</a></p><p>Row 0</p><p>Row 1</p></main>`
	if got != want {
		t.Fatalf("body:\n%s\nwant:\n%s", got, want)
	}
}

func TestBodyLinkWithoutURL(t *testing.T) {
	got := string(Body(contentView, Options{}))
	if !strings.Contains(got, `<p>Docs</p>`) {
		t.Fatalf("body: %s", got)
	}
	if strings.Contains(got, `<a href=`) {
		t.Fatalf("body has a link: %s", got)
	}
}

func TestBodyEscapes(t *testing.T) {
	view := func(c *ui.Context) {
		ui.Text(c, `<b>bold</b> & more`)
	}
	got := string(Body(view, Options{}))
	want := `<main><p>&lt;b&gt;bold&lt;/b&gt; &amp; more</p></main>`
	if got != want {
		t.Fatalf("body:\n%s\nwant:\n%s", got, want)
	}
}

func TestBodyCompositeHeading(t *testing.T) {
	view := func(c *ui.Context) {
		ui.Column(c).Children(func() {
			ui.Column(c).Role(ui.RoleHeading).Level(2).Children(func() {
				ui.Text(c, "Composite")
				ui.Text(c, "Heading")
			})
			ui.Text(c, "After")
		})
	}
	got := string(Body(view, Options{}))
	want := `<main><h2>Composite Heading</h2><p>After</p></main>`
	if got != want {
		t.Fatalf("body:\n%s\nwant:\n%s", got, want)
	}
}

func TestBodyHeadingLevelDefaults(t *testing.T) {
	view := func(c *ui.Context) {
		ui.Text(c, "A heading").Role(ui.RoleHeading)
	}
	got := string(Body(view, Options{}))
	if !strings.Contains(got, `<h2>A heading</h2>`) {
		t.Fatalf("body: %s", got)
	}
}

func TestBodyEmpty(t *testing.T) {
	if body := Body(func(*ui.Context) {}, Options{}); body != nil {
		t.Fatalf("empty body: %s", body)
	}
	if body := Body(nil, Options{}); body != nil {
		t.Fatalf("nil view body: %s", body)
	}
}

func TestBodyDeterministic(t *testing.T) {
	first := Body(contentView, Options{Links: map[string]string{"Docs": "https://example.com/docs"}})
	second := Body(contentView, Options{Links: map[string]string{"Docs": "https://example.com/docs"}})
	if string(first) != string(second) {
		t.Fatalf("bodies differ:\n%s\n%s", first, second)
	}
}

func TestBodyViewport(t *testing.T) {
	narrow := Body(contentView, Options{Width: 200, Height: 200})
	wide := Body(contentView, Options{Width: 1280, Height: 800})
	if len(narrow) == 0 || len(wide) == 0 {
		t.Fatalf("narrow %d bytes, wide %d bytes", len(narrow), len(wide))
	}
}

func TestLinkRecordsItsURL(t *testing.T) {
	view := func(c *ui.Context) {
		ui.Column(c).Children(func() {
			ui.Text(c, "Home").Role(ui.RoleHeading).Level(1)
			Link(c, "Pricing", "/pricing")
			ui.Link(c, "Docs", "/docs")
		})
	}
	got := string(Body(view, Options{Links: map[string]string{"Docs": "/documentation"}}))
	for _, want := range []string{`<a href="/pricing">Pricing</a>`, `<a href="/documentation">Docs</a>`} {
		if !strings.Contains(got, want) {
			t.Fatalf("body misses %q: %s", want, got)
		}
	}
}

func TestRecordNeedsARender(t *testing.T) {
	record("Pricing", "/pricing")
	if recording != nil {
		t.Fatal("a link outside a render was recorded")
	}
}
