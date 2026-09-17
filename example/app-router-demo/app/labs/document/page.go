package document

import (
	"net/http"

	"github.com/3-lines-studio/bifrost"
)

func Load(r *http.Request) (any, error) {
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "pt-BR"
	}
	return bifrost.PageData{
		Props:    map[string]any{"lang": lang},
		Document: bifrost.Document{Lang: lang, Class: "lab-dark", Dir: "ltr"},
	}, nil
}
