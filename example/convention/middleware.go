package convention

import (
	"context"
	"net/http"
)

type ContextKey string

const MiddlewareOrder ContextKey = "middleware-order"

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order := []string{"root"}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), MiddlewareOrder, order)))
	})
}
