package bifrost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"maps"
	"reflect"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

const bodyChunkSize = 32 << 10

type templateRenderer struct {
	page           *template.Template
	layouts        []*template.Template
	errors         []*template.Template
	notFound       *template.Template
	componentCSS   []string
	componentJS    map[string]string
	hasComponentJS bool
}

type componentRenderer struct {
	template *template.Template
	defaults map[string]any
	required []string
}

type templateData struct {
	Props        map[string]any
	Params       map[string]any
	SearchParams map[string]any
	Pathname     string
	Content      template.HTML
}

func newTemplateRenderer(assets *protocol.HTMLAssets, funcs ...template.FuncMap) (*templateRenderer, error) {
	if assets == nil {
		return nil, errors.New("bifrost: html view has no templates")
	}
	sources := make([]protocol.TemplateSource, 0, 1+len(assets.Layouts)+len(assets.Errors))
	sources = append(sources, assets.Page)
	sources = append(sources, assets.Layouts...)
	sources = append(sources, assets.Errors...)
	if assets.NotFound != nil {
		sources = append(sources, *assets.NotFound)
	}
	used := usedComponentNames(sources, assets.Components)
	components := make(map[string]componentRenderer)
	funcMap := template.FuncMap{}
	if len(funcs) > 0 {
		for name, function := range funcs[0] {
			funcMap[name] = function
		}
	}
	funcMap["dict"] = componentDict
	funcMap["list"] = func(values ...any) []any { return values }
	funcMap["component"] = func(name string, data any, tags ...string) (template.HTML, error) {
		component, exists := components[name]
		if !exists {
			return "", fmt.Errorf("bifrost: component %q not found", name)
		}
		tag := "div"
		if len(tags) > 0 {
			tag = tags[0]
		}
		if len(tags) > 1 || !validComponentTag(tag) {
			return "", fmt.Errorf("bifrost: invalid wrapper tag %q", tag)
		}
		props, err := componentProps(name, component.defaults, component.required, data)
		if err != nil {
			return "", err
		}
		var body bytes.Buffer
		if err := component.template.Execute(&body, props); err != nil {
			return "", err
		}
		return template.HTML("<" + tag + ` data-bifrost-component="` + template.HTMLEscapeString(name) + `">` + body.String() + "</" + tag + ">"), nil
	}
	page, err := parseTemplate(assets.Page.Path, assets.Page.Source, funcMap)
	if err != nil {
		return nil, err
	}
	renderer := &templateRenderer{page: page, componentJS: make(map[string]string)}
	for _, layout := range assets.Layouts {
		parsed, err := parseTemplate(layout.Path, layout.Source, funcMap)
		if err != nil {
			return nil, err
		}
		renderer.layouts = append(renderer.layouts, parsed)
	}
	for _, source := range assets.Errors {
		parsed, err := parseTemplate(source.Path, source.Source, funcMap)
		if err != nil {
			return nil, err
		}
		renderer.errors = append(renderer.errors, parsed)
	}
	if assets.NotFound != nil {
		parsed, err := parseTemplate(assets.NotFound.Path, assets.NotFound.Source, funcMap)
		if err != nil {
			return nil, err
		}
		renderer.notFound = parsed
	}
	for _, component := range assets.Components {
		parsed, err := parseTemplate(component.HTML.Path, component.HTML.Source, funcMap)
		if err != nil {
			return nil, err
		}
		components[component.Name] = componentRenderer{template: parsed, defaults: component.Defaults, required: component.Required}
		if _, exists := used[component.Name]; !exists {
			continue
		}
		if component.CSS != "" {
			renderer.componentCSS = append(renderer.componentCSS, component.CSS)
		}
		if component.JS != "" {
			renderer.hasComponentJS = true
			renderer.componentJS[component.Name] = componentURL(component)
		}
	}
	return renderer, nil
}

func componentDict(values ...any) (map[string]any, error) {
	if len(values)%2 != 0 {
		return nil, errors.New("bifrost: dict requires an even number of arguments")
	}
	result := make(map[string]any, len(values)/2)
	for index := 0; index < len(values); index += 2 {
		key, ok := values[index].(string)
		if !ok {
			return nil, fmt.Errorf("bifrost: dict key %v is not a string", values[index])
		}
		result[key] = values[index+1]
	}
	return result, nil
}

func componentProps(name string, defaults map[string]any, required []string, data any) (any, error) {
	if defaults == nil && len(required) == 0 {
		return data, nil
	}
	props := make(map[string]any, len(defaults))
	maps.Copy(props, defaults)
	if err := mergeComponentProps(props, data); err != nil {
		return nil, fmt.Errorf("bifrost: component %q: %w", name, err)
	}
	for _, key := range required {
		if _, exists := props[key]; !exists {
			return nil, fmt.Errorf("bifrost: component %q: missing required prop %q", name, key)
		}
	}
	return props, nil
}

func mergeComponentProps(props map[string]any, data any) error {
	if data == nil {
		return nil
	}
	value := reflect.ValueOf(data)
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Map:
		iterator := value.MapRange()
		for iterator.Next() {
			key := iterator.Key()
			if key.Kind() != reflect.String {
				return fmt.Errorf("props map keys must be strings")
			}
			props[key.String()] = iterator.Value().Interface()
		}
	case reflect.Struct:
		typ := value.Type()
		for index := 0; index < value.NumField(); index++ {
			field := typ.Field(index)
			if field.PkgPath != "" {
				continue
			}
			props[field.Name] = value.Field(index).Interface()
		}
	default:
		return fmt.Errorf("props must be a map or struct, got %s", value.Kind())
	}
	return nil
}

func parseTemplate(name, source string, funcs template.FuncMap) (*template.Template, error) {
	base := template.New(name)
	if funcs != nil {
		base = base.Funcs(funcs)
	}
	parsed, err := base.Parse(source)
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

	view := r.page
	if notFoundRequested(props) && r.notFound != nil {
		view = r.notFound
	} else if errorRequested(props) && len(r.errors) > 0 {
		view = r.errors[errorIndex(props, len(r.errors))]
	}

	var head bytes.Buffer
	for _, layout := range r.layouts {
		if layout.Lookup("head") == nil {
			continue
		}
		if err := layout.ExecuteTemplate(&head, "head", data); err != nil {
			return err
		}
	}
	if view.Lookup("head") != nil {
		if err := view.ExecuteTemplate(&head, "head", data); err != nil {
			return err
		}
	}
	if len(r.componentCSS) > 0 {
		head.WriteString(`<style data-bifrost-components>`)
		for _, css := range r.componentCSS {
			head.WriteString(css)
		}
		head.WriteString(`</style>`)
	}
	if r.hasComponentJS {
		head.WriteString(`<script type="module" src="` + componentRuntimePath + `"></script>`)
	}

	var body bytes.Buffer
	if err := view.Execute(&body, data); err != nil {
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
	if len(r.componentJS) > 0 {
		encoded, err := json.Marshal(r.componentJS)
		if err != nil {
			return err
		}
		body.WriteString(`<script type="application/json" id="bifrost-component-manifest">`)
		body.Write(encoded)
		body.WriteString(`</script>`)
	}
	if err := sink.Head(head.Bytes()); err != nil {
		return err
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

func notFoundRequested(props map[string]any) bool {
	value, ok := props["__bifrostNotFound"].(bool)
	return ok && value
}

func errorRequested(props map[string]any) bool {
	_, ok := props["__bifrostError"]
	return ok
}

func errorIndex(props map[string]any, count int) int {
	if count == 0 {
		return 0
	}
	switch value := props["__bifrostErrorLevel"].(type) {
	case float64:
		index := int(value)
		if index >= 0 && index < count {
			return index
		}
	case int:
		if value >= 0 && value < count {
			return value
		}
	}
	return count - 1
}
