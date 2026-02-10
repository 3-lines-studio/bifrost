package bifrost

import "net/http"

type renderedPage struct {
	Body string
	Head string
}

type propsLoader func(*http.Request) (map[string]interface{}, error)

type PageOption func(*options)

type options struct {
	ComponentPath      string
	PropsLoader        propsLoader
	EntryDir           string
	Outdir             string
	PublicDir          string
	Title              string
	ErrorComponentPath string
	Static             bool
}

type RedirectError interface {
	RedirectURL() string
	RedirectStatusCode() int
}

type manifestEntry struct {
	Script string   `json:"script"`
	CSS    string   `json:"css,omitempty"`
	Chunks []string `json:"chunks,omitempty"`
	Static bool     `json:"static,omitempty"`
}

type buildManifest struct {
	Entries map[string]manifestEntry `json:"entries"`
	Chunks  map[string]string        `json:"chunks,omitempty"`
}

type Router interface {
	Handle(pattern string, handler http.Handler)
}

func WithStatic() PageOption {
	return func(opts *options) {
		opts.Static = true
	}
}

func WithTitle(title string) PageOption {
	return func(opts *options) {
		opts.Title = title
	}
}
