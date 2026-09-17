package demo

import (
	"context"
	"net/http"
	"strings"
)

type ContextKey string

const OrderKey ContextKey = "demo-middleware-order"

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order := []string{"root"}
		w.Header().Set("X-Demo-Root", "1")
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), OrderKey, order)))
	})
}

func Order(r *http.Request) []string {
	order, _ := r.Context().Value(OrderKey).([]string)
	return order
}

func Push(r *http.Request, name string) []string {
	return append(Order(r), name)
}

func PathPrefix(r *http.Request) string {
	return strings.TrimPrefix(r.URL.Path, "/")
}
