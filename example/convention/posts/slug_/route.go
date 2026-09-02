package slug

import (
	"encoding/json"
	"net/http"

	app "github.com/3-lines-studio/bifrost/example/convention"
)

func Delete(w http.ResponseWriter, r *http.Request) {
	order, _ := r.Context().Value(app.MiddlewareOrder).([]string)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"method":     r.Method,
		"slug":       r.PathValue("slug"),
		"middleware": order,
	})
}

func Post(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
	Delete(w, r)
}
