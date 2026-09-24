package admin

import (
	"context"
	"net/http"

	app "github.com/3-lines-studio/bifrost/example/app-router-demo/app"
)

const Cookie = "demo-admin"

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Demo-Admin", "1")
		request := r.WithContext(context.WithValue(r.Context(), app.OrderKey, app.Push(r, "admin")))
		if r.URL.Path == "/admin/login" || r.URL.Path == "/admin/login/" {
			next.ServeHTTP(w, request)
			return
		}
		if cookie, err := r.Cookie(Cookie); err == nil && cookie.Value == "ok" {
			next.ServeHTTP(w, request)
			return
		}
		target := "/admin/login?next=" + r.URL.EscapedPath()
		http.Redirect(w, request, target, http.StatusFound)
	})
}
