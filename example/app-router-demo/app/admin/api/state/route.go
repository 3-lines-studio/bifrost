package state

import (
	"encoding/json"
	"net/http"

	app "github.com/3-lines-studio/bifrost/example/app-router-demo/app"
)

func Get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"middleware": app.Order(r),
		"cookie":     r.Header.Get("Cookie"),
		"method":     r.Method,
	})
}
