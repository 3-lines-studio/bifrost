package bifrost

import (
	"embed"
	"net/http"
	"path/filepath"
	"strings"
)

func AssetHandler() http.Handler {
	return http.FileServer(http.Dir(".bifrost"))
}

func EmbeddedAssetHandler(assetsFS embed.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, "/")
		if path == "" {
			http.NotFound(w, req)
			return
		}

		embedPath := filepath.Join(".bifrost", path)
		embedPath = filepath.ToSlash(embedPath)

		data, err := assetsFS.ReadFile(embedPath)
		if err != nil {
			http.NotFound(w, req)
			return
		}

		contentType := getContentType(path)
		w.Header().Set("Content-Type", contentType)
		w.Write(data)
	})
}

func PublicHandler(assetsFS embed.FS, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, "/")
		if path == "" {
			next.ServeHTTP(w, req)
			return
		}

		var exists bool
		if IsDev() {
			exists = publicFileExistsInFS(path)
		} else {
			exists = publicFileExistsInEmbed(assetsFS, path)
		}

		if !exists {
			next.ServeHTTP(w, req)
			return
		}

		if IsDev() {
			servePublicFromFS(w, req, path)
		} else {
			servePublicFromEmbed(assetsFS, w, req, path)
		}
	})
}
