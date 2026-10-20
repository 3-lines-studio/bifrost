package timeapi

import (
	"encoding/json"
	"net/http"
	"time"
)

func Get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"time": time.Now().Format(time.RFC3339)})
}
