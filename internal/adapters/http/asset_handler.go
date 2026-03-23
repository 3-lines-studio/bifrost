package http

import (
	"embed"
	"io"
	"io/fs"
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

	if err := serveBifrostFile(w, req, h.assetsFS, cleaned, !h.isDev, core.GetContentType(cleaned)); err != nil {
		http.NotFound(w, req)
	}
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

	if err := serveProjectFile(w, req, h.assetsFS, "public", cleaned, !h.isDev, core.GetContentType(cleaned)); err != nil {
		h.next.ServeHTTP(w, req)
	}
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

func serveBifrostFile(w http.ResponseWriter, req *http.Request, assetsFS embed.FS, cleaned string, fromEmbed bool, contentType string) error {
	return serveProjectFile(w, req, assetsFS, ".bifrost", cleaned, fromEmbed, contentType)
}

func serveProjectFile(w http.ResponseWriter, req *http.Request, assetsFS embed.FS, root string, cleaned string, fromEmbed bool, contentType string) error {
	if fromEmbed {
		return serveFileFromEmbed(w, req, assetsFS, path.Join(root, cleaned), contentType)
	}
	return serveFileFromDisk(w, req, filepath.Join(root, cleaned), root, contentType)
}

func serveFileFromDisk(w http.ResponseWriter, req *http.Request, fullPath string, root string, contentType string) error {
	if !isPathSafe(fullPath, root) {
		return os.ErrNotExist
	}

	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		return os.ErrNotExist
	}

	w.Header().Set("Content-Type", contentType)
	http.ServeFile(w, req, fullPath)
	return nil
}

func serveFileFromEmbed(w http.ResponseWriter, req *http.Request, assetsFS embed.FS, embedPath string, contentType string) error {
	file, err := assetsFS.Open(embedPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return os.ErrNotExist
	}

	seeker, ok := file.(io.ReadSeeker)
	if !ok {
		return fs.ErrInvalid
	}

	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, req, info.Name(), info.ModTime(), seeker)
	return nil
}
