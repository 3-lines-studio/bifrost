package slug

import (
	"net/http"

	"github.com/3-lines-studio/bifrost"
	app "github.com/3-lines-studio/bifrost/example/app-router-demo/app"
	store "github.com/3-lines-studio/bifrost/example/app-router-demo/app/store"
)

func Load(r *http.Request) (any, error) {
	path := r.PathValue("slug")
	doc, ok := store.FindDoc(path)
	if !ok {
		return bifrost.PageData{
			Props:  map[string]any{"missing": path, "docs": store.DocPaths()},
			Status: http.StatusNotFound,
		}, nil
	}
	return map[string]any{
		"title":      doc.Title,
		"body":       doc.Body,
		"docs":       store.DocPaths(),
		"middleware": app.Order(r),
		"loads":      store.Loads("/docs/" + path),
	}, nil
}
