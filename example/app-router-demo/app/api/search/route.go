package search

import (
	"encoding/json"
	"net/http"
	"strings"

	store "github.com/3-lines-studio/bifrost/example/app-router-demo/app/store"
)

func Get(w http.ResponseWriter, r *http.Request) {
	query := strings.ToLower(r.URL.Query().Get("q"))
	hits := []string{}
	for _, path := range store.DocPaths() {
		doc, _ := store.FindDoc(path)
		if query == "" || strings.Contains(strings.ToLower(doc.Title+" "+doc.Body), query) {
			hits = append(hits, "/docs/"+path)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"query": query, "count": len(hits), "hits": hits})
}
