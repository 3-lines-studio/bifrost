package bifrost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

type templateTestSink struct {
	head    bytes.Buffer
	body    bytes.Buffer
	started bool
}

func (s *templateTestSink) Head(data []byte) error {
	if s.started {
		return errors.New("head emitted twice")
	}
	s.started = true
	s.head.Write(data)
	return nil
}

func (s *templateTestSink) Body(data []byte) error {
	if !s.started {
		return errors.New("body emitted before head")
	}
	s.body.Write(data)
	return nil
}

func TestTemplateRendererComposesLayoutsAndHead(t *testing.T) {
	renderer, err := newTemplateRenderer(&protocol.HTMLAssets{
		Page: protocol.TemplateSource{
			Path:   "app/page.html",
			Source: `{{define "head"}}<title>{{.Props.title}}</title>{{end}}<h1>{{.Props.title}}</h1>`,
		},
		Layouts: []protocol.TemplateSource{
			{Path: "app/layout.html", Source: `{{define "head"}}<meta name="site" content="bifrost">{{end}}<main>{{.Content}}</main>`},
			{Path: "app/blog/layout.html", Source: `<article>{{.Content}}</article>`},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	props, err := json.Marshal(map[string]any{"title": "Hola"})
	if err != nil {
		t.Fatal(err)
	}
	sink := &templateTestSink{}
	if err := renderer.Render(context.Background(), renderRequest{Props: props}, sink); err != nil {
		t.Fatal(err)
	}
	if got, want := sink.head.String(), `<meta name="site" content="bifrost"><title>Hola</title>`; got != want {
		t.Fatalf("head = %q, want %q", got, want)
	}
	if got, want := sink.body.String(), `<main><article><h1>Hola</h1></article></main>`; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestTemplateRendererExposesParamsAndEscapesProps(t *testing.T) {
	renderer, err := newTemplateRenderer(&protocol.HTMLAssets{
		Page: protocol.TemplateSource{
			Path:   "app/blog/slug_/page.html",
			Source: `<p>{{.Params.slug}} {{.Props.pathname}}</p><div>{{.Props.body}}</div>`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	props, err := json.Marshal(map[string]any{
		"body":         `<b>unsafe</b>`,
		"params":       map[string]any{"slug": "hola"},
		"pathname":     "/blog/hola",
		"search":       "value",
		"searchParams": map[string]any{"q": "go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	sink := &templateTestSink{}
	if err := renderer.Render(context.Background(), renderRequest{Props: props}, sink); err != nil {
		t.Fatal(err)
	}
	want := `<p>hola /blog/hola</p><div>&lt;b&gt;unsafe&lt;/b&gt;</div>`
	if got := sink.body.String(); got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestTemplateRendererRejectsInvalidInput(t *testing.T) {
	if _, err := newTemplateRenderer(nil); err == nil {
		t.Fatal("nil assets were accepted")
	}
	if _, err := newTemplateRenderer(&protocol.HTMLAssets{Page: protocol.TemplateSource{Path: "page.html", Source: "{{"}}); err == nil {
		t.Fatal("invalid template was accepted")
	}
	renderer, err := newTemplateRenderer(&protocol.HTMLAssets{Page: protocol.TemplateSource{Path: "page.html", Source: "ok"}})
	if err != nil {
		t.Fatal(err)
	}
	sink := &templateTestSink{}
	if err := renderer.Render(context.Background(), renderRequest{Props: json.RawMessage("{")}, sink); err == nil {
		t.Fatal("invalid props were accepted")
	}
}

func TestTemplateRendererStreamsBodyChunks(t *testing.T) {
	renderer, err := newTemplateRenderer(&protocol.HTMLAssets{
		Page: protocol.TemplateSource{Path: "page.html", Source: strings.Repeat("x", bodyChunkSize+10)},
	})
	if err != nil {
		t.Fatal(err)
	}
	sink := &templateTestSink{}
	if err := renderer.Render(context.Background(), renderRequest{}, sink); err != nil {
		t.Fatal(err)
	}
	if sink.body.Len() != bodyChunkSize+10 {
		t.Fatalf("body length = %d", sink.body.Len())
	}
}

func TestTemplateRendererSelectsErrorAndNotFoundViews(t *testing.T) {
	notFound := protocol.TemplateSource{Path: "app/not-found.html", Source: `{{define "head"}}<title>missing</title>{{end}}<p>missing</p>`}
	renderer, err := newTemplateRenderer(&protocol.HTMLAssets{
		Page:    protocol.TemplateSource{Path: "app/page.html", Source: `{{define "head"}}<title>page</title>{{end}}<h1>page</h1>`},
		Layouts: []protocol.TemplateSource{{Path: "app/layout.html", Source: `<main>{{.Content}}</main>`}},
		Errors: []protocol.TemplateSource{
			{Path: "app/error.html", Source: `{{define "head"}}<title>root error</title>{{end}}<p>root</p>`},
			{Path: "app/blog/error.html", Source: `{{define "head"}}<title>blog error</title>{{end}}<p>blog</p>`},
		},
		NotFound: &notFound,
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		props map[string]any
		head  string
		body  string
	}{
		{"blog error", map[string]any{"__bifrostError": "boom", "__bifrostErrorLevel": 1}, `<title>blog error</title>`, `<main><p>blog</p></main>`},
		{"nearest error", map[string]any{"__bifrostError": "boom"}, `<title>blog error</title>`, `<main><p>blog</p></main>`},
		{"root error", map[string]any{"__bifrostError": "boom", "__bifrostErrorLevel": 0}, `<title>root error</title>`, `<main><p>root</p></main>`},
		{"not found", map[string]any{"__bifrostNotFound": true}, `<title>missing</title>`, `<main><p>missing</p></main>`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			props, err := json.Marshal(test.props)
			if err != nil {
				t.Fatal(err)
			}
			sink := &templateTestSink{}
			if err := renderer.Render(context.Background(), renderRequest{Props: props}, sink); err != nil {
				t.Fatal(err)
			}
			if got := sink.head.String(); got != test.head {
				t.Fatalf("head = %q, want %q", got, test.head)
			}
			if got := sink.body.String(); got != test.body {
				t.Fatalf("body = %q, want %q", got, test.body)
			}
			if strings.Contains(sink.head.String(), "page") || strings.Contains(sink.body.String(), "page</h1>") {
				t.Fatalf("page rendered on %s", test.name)
			}
		})
	}
}
