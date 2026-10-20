package app

import "net/http"

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Go-Page", "1")
		next.ServeHTTP(w, r)
	})
}
