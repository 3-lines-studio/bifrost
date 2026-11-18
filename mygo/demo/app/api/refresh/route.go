// Package refresh trades a token for another one, which is what the app calls
// when the server rejects the token it holds.
package refresh

import (
	"encoding/json"
	"net/http"

	"github.com/3-lines-studio/bifrost/mygo/demo/session"
)

// Post answers a new token for the one the request carries.
func Post(w http.ResponseWriter, r *http.Request) {
	email, ok := session.Email(session.Bearer(r.Header.Get("Authorization")))
	if !ok {
		answer(w, http.StatusUnauthorized, map[string]string{"error": "no token to renew"})
		return
	}
	answer(w, http.StatusOK, map[string]string{"token": session.Issue(email)})
}

func answer(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
