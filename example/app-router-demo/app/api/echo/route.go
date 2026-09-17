package echo

import (
	"encoding/json"
	"net/http"
)

func respond(w http.ResponseWriter, r *http.Request) {
	body, _ := json.Marshal(map[string]string{"method": r.Method, "pattern": "/api/echo"})
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func Get(w http.ResponseWriter, r *http.Request)    { respond(w, r) }
func Post(w http.ResponseWriter, r *http.Request)   { respond(w, r) }
func Put(w http.ResponseWriter, r *http.Request)    { respond(w, r) }
func Patch(w http.ResponseWriter, r *http.Request)  { respond(w, r) }
func Delete(w http.ResponseWriter, r *http.Request) { respond(w, r) }
func Options(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Allow", "GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}

func Head(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}
