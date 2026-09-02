package api

import (
	"encoding/json"
	"net/http"

	app "github.com/3-lines-studio/bifrost/example/convention"
)

func Get(w http.ResponseWriter, r *http.Request) {
	respond(w, r)
}

func Post(w http.ResponseWriter, r *http.Request) {
	respond(w, r)
}

func Put(w http.ResponseWriter, r *http.Request) {
	respond(w, r)
}

func Patch(w http.ResponseWriter, r *http.Request) {
	respond(w, r)
}

func Delete(w http.ResponseWriter, r *http.Request) {
	respond(w, r)
}

func Head(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Route-Method", r.Method)
}

func Options(w http.ResponseWriter, r *http.Request) {
	respond(w, r)
}

func respond(w http.ResponseWriter, r *http.Request) {
	order, _ := r.Context().Value(app.MiddlewareOrder).([]string)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"method":     r.Method,
		"slug":       r.PathValue("slug"),
		"middleware": order,
	})
}
