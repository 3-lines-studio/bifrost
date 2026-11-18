// Package notes is the API the dashboard reads and changes.
package notes

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/3-lines-studio/bifrost/mygo/demo/store"
)

// Get answers the notes.
func Get(w http.ResponseWriter, _ *http.Request) {
	write(w, http.StatusOK, store.Notes())
}

// Post keeps a note and answers it.
func Post(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		write(w, http.StatusBadRequest, map[string]string{"error": "the body is not JSON"})
		return
	}
	if strings.TrimSpace(input.Title) == "" {
		write(w, http.StatusBadRequest, map[string]string{"error": "a note needs a title"})
		return
	}
	write(w, http.StatusCreated, store.AddNote(input.Title))
}

func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
