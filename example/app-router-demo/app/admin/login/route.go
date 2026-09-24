package login

import (
	"net/http"
	"strings"

	admin "github.com/3-lines-studio/bifrost/example/app-router-demo/app/admin"
)

func Post(w http.ResponseWriter, r *http.Request) {
	next := r.URL.Query().Get("next")
	if !strings.HasPrefix(next, "/") {
		next = "/admin"
	}
	http.SetCookie(w, &http.Cookie{Name: admin.Cookie, Value: "ok", Path: "/", SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, next, http.StatusSeeOther)
}
