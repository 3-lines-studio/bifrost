package http

import (
	"embed"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/core"
)

type AssetHandler struct {
	assetsFS embed.FS
	isDev    bool
}

func NewAssetHandler(assetsFS embed.FS, isDev bool) http.Handler {
	return &AssetHandler{
		assetsFS: assetsFS,
		isDev:    isDev,
	}
}

func (h *AssetHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/")
	if path == "" {
		http.NotFound(w, req)
		return
	}

	if h.isDev {
		h.serveFromFS(w, req, path)
	} else {
		h.serveFromEmbed(w, req, path)
	}
}

func (h *AssetHandler) serveFromFS(w http.ResponseWriter, req *http.Request, path string) {
	fullPath := filepath.Join(".bifrost", path)

	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		http.NotFound(w, req)
		return
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		http.NotFound(w, req)
		return
	}

	contentType := core.GetContentType(path)
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}

func (h *AssetHandler) serveFromEmbed(w http.ResponseWriter, req *http.Request, path string) {
	embedPath := filepath.Join(".bifrost", path)
	embedPath = filepath.ToSlash(embedPath)

	data, err := h.assetsFS.ReadFile(embedPath)
	if err != nil {
		http.NotFound(w, req)
		return
	}

	contentType := core.GetContentType(path)
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}

type PublicHandler struct {
	assetsFS embed.FS
	next     http.Handler
	isDev    bool
}

func NewPublicHandler(assetsFS embed.FS, next http.Handler, isDev bool) http.Handler {
	return &PublicHandler{
		assetsFS: assetsFS,
		next:     next,
		isDev:    isDev,
	}
}

func (h *PublicHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/")
	if path == "" {
		h.next.ServeHTTP(w, req)
		return
	}

	publicPath := filepath.Join("public", path)

	var exists bool
	if h.isDev {
		exists = h.fileExistsInFS(publicPath)
	} else {
		exists = h.fileExistsInEmbed(publicPath)
	}

	if !exists {
		h.next.ServeHTTP(w, req)
		return
	}

	if h.isDev {
		h.servePublicFromFS(w, req, publicPath)
	} else {
		h.servePublicFromEmbed(w, req, publicPath)
	}
}

func (h *PublicHandler) fileExistsInFS(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (h *PublicHandler) fileExistsInEmbed(path string) bool {
	embedPath := filepath.ToSlash(path)
	_, err := h.assetsFS.ReadFile(embedPath)
	return err == nil
}

func (h *PublicHandler) servePublicFromFS(w http.ResponseWriter, req *http.Request, path string) {
	info, err := os.Stat(path)
	if err != nil {
		http.NotFound(w, req)
		return
	}

	if info.IsDir() {
		http.NotFound(w, req)
		return
	}

	file, err := os.Open(path)
	if err != nil {
		http.NotFound(w, req)
		return
	}
	defer func() { _ = file.Close() }()

	contentType := core.GetContentType(path)
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, req, info.Name(), info.ModTime(), file)
}

func (h *PublicHandler) servePublicFromEmbed(w http.ResponseWriter, req *http.Request, path string) {
	embedPath := filepath.ToSlash(path)
	data, err := h.assetsFS.ReadFile(embedPath)
	if err != nil {
		http.NotFound(w, req)
		return
	}

	contentType := core.GetContentType(path)
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}
