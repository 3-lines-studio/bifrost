package http

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
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

	if !h.isDev {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	if err := serveBifrostFile(w, req, h.assetsFS, cleaned, !h.isDev, core.GetContentType(cleaned)); err != nil {
		w.Header().Del("Cache-Control")
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

	root := "public"
	if !h.isDev {
		root = ".bifrost/public"
	}
	if err := serveProjectFile(w, req, h.assetsFS, root, cleaned, !h.isDev, core.GetContentType(cleaned)); err != nil {
		h.next.ServeHTTP(w, req)
	}
}

func serveBifrostFile(w http.ResponseWriter, req *http.Request, assetsFS embed.FS, cleaned string, fromEmbed bool, contentType string) error {
	return serveProjectFile(w, req, assetsFS, ".bifrost", cleaned, fromEmbed, contentType)
}

func serveProjectFile(w http.ResponseWriter, req *http.Request, assetsFS embed.FS, root string, cleaned string, fromEmbed bool, contentType string) error {
	if fromEmbed {
		return serveFileFromEmbed(w, req, assetsFS, path.Join(root, cleaned), contentType)
	}
	return serveFileFromDisk(w, req, root, cleaned, contentType)
}

func serveFileFromDisk(w http.ResponseWriter, req *http.Request, rootPath string, cleaned string, contentType string) error {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	file, err := root.Open(cleaned)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return os.ErrNotExist
	}

	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, req, info.Name(), info.ModTime(), file)
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
