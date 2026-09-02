package posts

import (
	"context"
	"net/http"

	app "github.com/3-lines-studio/bifrost/example/convention"
)

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order, _ := r.Context().Value(app.MiddlewareOrder).([]string)
		order = append(order, "posts")
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), app.MiddlewareOrder, order)))
	})
}
