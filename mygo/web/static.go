package web

import (
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Static serves the files of fsys, as the favicons, the manifest, the social
// cards and the pictures of a site, with the cache header a deploy wants:
// everyone holds them maxAge, and nobody asks again before.
//
//	mux.Handle("GET /static/", http.StripPrefix("/static/", web.Static(assets, 24*time.Hour)))
//
// A name that carries a version, as "app.4f2c1a.css", is worth a year; a name
// that never changes, an hour. A file that is not there answers 404, and the
// directory listing of a folder is not served.
func Static(fsys fs.FS, maxAge time.Duration) http.Handler {
	files := http.FileServerFS(fsys)
	header := "public, max-age=" + strconv.Itoa(int(maxAge.Seconds()))
	if maxAge <= 0 {
		header = "no-cache"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", header)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}
