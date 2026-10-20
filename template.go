package bifrost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

const bodyChunkSize = 32 << 10

type templateRenderer struct {
	page    *template.Template
	layouts []*template.Template
	head    []*template.Template
}

type templateData struct {
	Props        map[string]any
	Params       map[string]any
	SearchParams map[string]any
	Pathname     string
	Content      template.HTML
}

func newTemplateRenderer(assets *protocol.HTMLAssets) (*templateRenderer, error) {
	if assets == nil {
		return nil, errors.New("bifrost: html view has no templates")
	}
	page, err := parseTemplate(assets.Page.Path, assets.Page.Source)
	if err != nil {
		return nil, err
	}
	renderer := &templateRenderer{page: page}
	for _, layout := range assets.Layouts {
		parsed, err := parseTemplate(layout.Path, layout.Source)
		if err != nil {
			return nil, err
		}
		renderer.layouts = append(renderer.layouts, parsed)
	}
	for _, source := range renderer.layouts {
		if source.Lookup("head") != nil {
			renderer.head = append(renderer.head, source)
		}
	}
	if page.Lookup("head") != nil {
		renderer.head = append(renderer.head, page)
	}
	return renderer, nil
}

func parseTemplate(name, source string) (*template.Template, error) {
	parsed, err := template.New(name).Parse(source)
	if err != nil {
		return nil, fmt.Errorf("bifrost: parse %s: %w", name, err)
	}
	return parsed, nil
}

func (r *templateRenderer) Render(ctx context.Context, request renderRequest, sink renderSink) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	props := map[string]any{}
	if len(request.Props) > 0 {
		if err := json.Unmarshal(request.Props, &props); err != nil {
			return fmt.Errorf("bifrost: decode props: %w", err)
		}
	}
	data := templateData{
		Props:        props,
		Params:       map[string]any{},
		SearchParams: map[string]any{},
	}
	if params, ok := props["params"].(map[string]any); ok {
		data.Params = params
	}
	if searchParams, ok := props["searchParams"].(map[string]any); ok {
		data.SearchParams = searchParams
	}
	if pathname, ok := props["pathname"].(string); ok {
		data.Pathname = pathname
	}

	var head bytes.Buffer
	for _, source := range r.head {
		if err := source.ExecuteTemplate(&head, "head", data); err != nil {
			return err
		}
	}
	if err := sink.Head(head.Bytes()); err != nil {
		return err
	}

	var body bytes.Buffer
	if err := r.page.Execute(&body, data); err != nil {
		return err
	}
	for index := len(r.layouts) - 1; index >= 0; index-- {
		if err := ctx.Err(); err != nil {
			return err
		}
		data.Content = template.HTML(body.String())
		body.Reset()
		if err := r.layouts[index].Execute(&body, data); err != nil {
			return err
		}
	}
	for start := 0; start < body.Len(); start += bodyChunkSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := start + bodyChunkSize
		if end > body.Len() {
			end = body.Len()
		}
		if err := sink.Body(body.Bytes()[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (r *templateRenderer) Close(context.Context) error { return nil }
