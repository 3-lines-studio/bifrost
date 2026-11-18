// Package me answers who the token says the app is. Its Middleware guards the
// endpoint, so a request without a token never reaches Get.
package me

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/3-lines-studio/bifrost/mygo/demo/session"
)

type emailKey struct{}

// Middleware refuses a request without a token, and hands the email of the one
// that has it to Get.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		email, ok := session.Email(session.Bearer(r.Header.Get("Authorization")))
		if !ok {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "sign in first"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), emailKey{}, email)))
	})
}

// Get answers the email the token was issued for.
func Get(w http.ResponseWriter, r *http.Request) {
	email, _ := r.Context().Value(emailKey{}).(string)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]string{"email": email})
}
