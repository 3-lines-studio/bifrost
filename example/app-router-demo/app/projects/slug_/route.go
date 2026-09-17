package slug

import (
	"encoding/json"
	"net/http"

	store "github.com/3-lines-studio/bifrost/example/app-router-demo/app/store"
)

func Post(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	project, err := store.AddLog(slug, "route.go", r.FormValue("text"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if r.Header.Get("Accept") == "application/json" {
		_ = json.NewEncoder(w).Encode(project)
		return
	}
	http.Redirect(w, r, "/projects/"+slug+"/log", http.StatusSeeOther)
}

func Put(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]string{"method": "PUT", "slug": r.PathValue("slug")})
}

func Patch(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]string{"method": "PATCH", "slug": r.PathValue("slug")})
}

func Delete(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]string{"method": "DELETE", "slug": r.PathValue("slug")})
}

func Options(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Allow", "POST, PUT, PATCH, DELETE, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}
