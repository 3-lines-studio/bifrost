package http

import (
	"embed"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/core"
)

// cleanPath sanitizes a URL path segment to prevent directory traversal.
// Returns the cleaned path and false if the path is unsafe (escapes root).
func cleanPath(raw string) (string, bool) {
	raw = strings.ReplaceAll(raw, "\\", "/")
	// Reject any raw input containing ".." segments before cleaning
	if containsDotDot(raw) {
		return "", false
	}
	cleaned := path.Clean("/" + raw)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" || cleaned == "." {
		return "", false
	}
	return cleaned, true
}

func containsDotDot(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

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
	cleaned, ok := cleanPath(req.URL.Path)
	if !ok {
		http.NotFound(w, req)
		return
	}

	if h.isDev {
		h.serveFromFS(w, req, cleaned)
	} else {
		h.serveFromEmbed(w, req, cleaned)
	}
}

func (h *AssetHandler) serveFromFS(w http.ResponseWriter, req *http.Request, p string) {
	fullPath := filepath.Join(".bifrost", p)

	abs, err := filepath.Abs(fullPath)
	if err != nil {
		http.NotFound(w, req)
		return
	}
	root, err := filepath.Abs(".bifrost")
	if err != nil {
		http.NotFound(w, req)
		return
	}
	if !strings.HasPrefix(abs, root+string(filepath.Separator)) && abs != root {
		http.NotFound(w, req)
		return
	}

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

	contentType := core.GetContentType(p)
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}

func (h *AssetHandler) serveFromEmbed(w http.ResponseWriter, req *http.Request, p string) {
	embedPath := path.Join(".bifrost", p)

	data, err := h.assetsFS.ReadFile(embedPath)
	if err != nil {
		http.NotFound(w, req)
		return
	}

	contentType := core.GetContentType(p)
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
	cleaned, ok := cleanPath(req.URL.Path)
	if !ok {
		h.next.ServeHTTP(w, req)
		return
	}

	publicPath := filepath.Join("public", cleaned)

	var exists bool
	if h.isDev {
		exists = h.fileExistsInFS(publicPath)
	} else {
		exists = h.fileExistsInEmbed(cleaned)
	}

	if !exists {
		h.next.ServeHTTP(w, req)
		return
	}

	if h.isDev {
		h.servePublicFromFS(w, req, publicPath)
	} else {
		h.servePublicFromEmbed(w, req, cleaned)
	}
}

func (h *PublicHandler) fileExistsInFS(p string) bool {
	abs, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	root, err := filepath.Abs("public")
	if err != nil {
		return false
	}
	if !strings.HasPrefix(abs, root+string(filepath.Separator)) && abs != root {
		return false
	}
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func (h *PublicHandler) fileExistsInEmbed(cleaned string) bool {
	embedPath := path.Join("public", cleaned)
	_, err := h.assetsFS.ReadFile(embedPath)
	return err == nil
}

func (h *PublicHandler) servePublicFromFS(w http.ResponseWriter, req *http.Request, p string) {
	abs, err := filepath.Abs(p)
	if err != nil {
		http.NotFound(w, req)
		return
	}
	root, err := filepath.Abs("public")
	if err != nil {
		http.NotFound(w, req)
		return
	}
	if !strings.HasPrefix(abs, root+string(filepath.Separator)) && abs != root {
		http.NotFound(w, req)
		return
	}

	info, err := os.Stat(p)
	if err != nil || info.IsDir() {
		http.NotFound(w, req)
		return
	}

	file, err := os.Open(p)
	if err != nil {
		http.NotFound(w, req)
		return
	}
	defer func() { _ = file.Close() }()

	contentType := core.GetContentType(p)
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, req, info.Name(), info.ModTime(), file)
}

func (h *PublicHandler) servePublicFromEmbed(w http.ResponseWriter, req *http.Request, cleaned string) {
	embedPath := path.Join("public", cleaned)
	data, err := h.assetsFS.ReadFile(embedPath)
	if err != nil {
		http.NotFound(w, req)
		return
	}

	contentType := core.GetContentType(cleaned)
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}
