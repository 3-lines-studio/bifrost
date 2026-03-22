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

func cleanPath(raw string) (string, bool) {
	raw = strings.ReplaceAll(raw, "\\", "/")
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

func safeEmbedPath(raw string) (string, bool) {
	rel, ok := cleanPath(raw)
	if !ok {
		return "", false
	}
	return path.Join(".bifrost", rel), true
}

func containsDotDot(p string) bool {
	for {
		idx := strings.Index(p, "..")
		if idx < 0 {
			return false
		}

		atStart := idx == 0 || p[idx-1] == '/'
		end := idx + 2
		atEnd := end == len(p) || p[end] == '/'
		if atStart && atEnd {
			return true
		}
		p = p[end:]
	}
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

	if !isPathSafe(fullPath, ".bifrost") {
		http.NotFound(w, req)
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		http.NotFound(w, req)
		return
	}

	contentType := core.GetContentType(p)
	w.Header().Set("Content-Type", contentType)
	http.ServeFile(w, req, fullPath)
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

	if h.isDev {
		h.servePublicFromFSDirect(w, req, cleaned)
	} else {
		h.servePublicFromEmbedDirect(w, req, cleaned)
	}
}

func (h *PublicHandler) servePublicFromFSDirect(w http.ResponseWriter, req *http.Request, cleaned string) {
	publicPath := filepath.Join("public", cleaned)

	if !isPathSafe(publicPath, "public") {
		h.next.ServeHTTP(w, req)
		return
	}

	info, err := os.Stat(publicPath)
	if err != nil || info.IsDir() {
		h.next.ServeHTTP(w, req)
		return
	}

	file, err := os.Open(publicPath)
	if err != nil {
		h.next.ServeHTTP(w, req)
		return
	}
	defer func() { _ = file.Close() }()

	contentType := core.GetContentType(publicPath)
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, req, info.Name(), info.ModTime(), file)
}

func (h *PublicHandler) servePublicFromEmbedDirect(w http.ResponseWriter, req *http.Request, cleaned string) {
	embedPath := path.Join("public", cleaned)
	data, err := h.assetsFS.ReadFile(embedPath)
	if err != nil {
		h.next.ServeHTTP(w, req)
		return
	}

	contentType := core.GetContentType(cleaned)
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}

func isPathSafe(p, root string) bool {
	abs, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	return abs == absRoot || strings.HasPrefix(abs, absRoot+string(filepath.Separator))
}
