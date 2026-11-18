// Package login trades credentials for a token.
package login

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/3-lines-studio/bifrost/mygo/demo/session"
)

// Post answers a token for the credentials of the body. Any password of four
// letters or more signs in; this stands in for a real check.
func Post(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		answer(w, http.StatusBadRequest, map[string]string{"error": "the body is not JSON"})
		return
	}
	if !strings.Contains(input.Email, "@") {
		answer(w, http.StatusUnauthorized, map[string]string{"error": "that is not an email address"})
		return
	}
	if len(input.Password) < 4 {
		answer(w, http.StatusUnauthorized, map[string]string{"error": "the password is too short"})
		return
	}
	answer(w, http.StatusOK, map[string]string{"token": session.Issue(input.Email)})
}

func answer(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
