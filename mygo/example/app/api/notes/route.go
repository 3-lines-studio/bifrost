// Package notes answers the notes the app reads.
package notes

import (
	"encoding/json"
	"net/http"

	"github.com/3-lines-studio/bifrost/mygo/example/store"
)

// Get answers GET /api/notes.
func Get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(store.All()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
