package projects

import (
	"context"
	"net/http"

	app "github.com/3-lines-studio/bifrost/example/app-router-demo/app"
)

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Demo-Projects", "1")
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), app.OrderKey, app.Push(r, "projects"))))
	})
}
