package bifrost

import "net/http"

type renderedPage struct {
	Body string
	Head string
}

type propsLoader func(*http.Request) (map[string]interface{}, error)

type options struct {
	ComponentPath      string
	PropsLoader        propsLoader
	EntryDir           string
	Outdir             string
	PublicDir          string
	Title              string
	Watch              bool
	WatchDir           string
	ErrorComponentPath string
}

type RedirectError interface {
	RedirectURL() string
	RedirectStatusCode() int
}

type manifestEntry struct {
	Script string   `json:"script"`
	CSS    string   `json:"css,omitempty"`
	Chunks []string `json:"chunks,omitempty"`
}

type buildManifest struct {
	Entries map[string]manifestEntry `json:"entries"`
	Chunks  map[string]string        `json:"chunks,omitempty"`
}

type Router interface {
	Handle(pattern string, handler http.Handler)
}
