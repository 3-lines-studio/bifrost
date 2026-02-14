package assets

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type ManifestEntry struct {
	Script       string            `json:"script"`
	CSS          string            `json:"css,omitempty"`
	Chunks       []string          `json:"chunks,omitempty"`
	Static       bool              `json:"static,omitempty"`
	SSR          string            `json:"ssr,omitempty"`
	Mode         string            `json:"mode,omitempty"`
	HTML         string            `json:"html,omitempty"`
	StaticRoutes map[string]string `json:"staticRoutes,omitempty"` // route -> html file path (for dynamic static pages)
}

type Manifest struct {
	Entries map[string]ManifestEntry `json:"entries"`
	Chunks  map[string]string        `json:"chunks,omitempty"`
}

func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseManifest(data)
}

func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func LoadManifestFromEmbed(fs embed.FS, path string) (*Manifest, error) {
	embedPath := filepath.ToSlash(path)
	data, err := fs.ReadFile(embedPath)
	if err != nil {
		return nil, err
	}
	return ParseManifest(data)
}

func GetAssets(man *Manifest, entryName string) (scriptSrc, cssHref string, chunks []string, isStatic bool, ssrPath string) {
	if man != nil && man.Entries[entryName].Script != "" {
		entry := man.Entries[entryName]
		return entry.Script, entry.CSS, entry.Chunks, entry.Static, entry.SSR
	}
	return "/dist/" + entryName + ".js", "/dist/" + entryName + ".css", nil, false, ""
}

// HasSSREntries returns true if the manifest contains any SSR pages.
// It checks both the Mode field (primary) and falls back to the SSR field for legacy manifests.
func HasSSREntries(man *Manifest) bool {
	if man == nil {
		return false
	}
	for _, entry := range man.Entries {
		if entry.Mode == "ssr" {
			return true
		}
		// Legacy fallback: check SSR bundle path for older manifests
		if entry.SSR != "" {
			return true
		}
	}
	return false
}

func EntryNameForPath(componentPath string) string {
	name := strings.TrimPrefix(componentPath, "./")
	name = strings.TrimPrefix(name, "/")
	name = strings.ReplaceAll(filepath.ToSlash(name), "/", "-")
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if name == "" {
		return "page-entry"
	}
	return name + "-entry"
}

type Resolver struct {
	AssetsFS   embed.FS
	Manifest   *Manifest
	IsDev      bool
	ssrTempDir string
}

func NewResolver(assetsFS embed.FS, manifest *Manifest, isDev bool) *Resolver {
	return &Resolver{
		AssetsFS:   assetsFS,
		Manifest:   manifest,
		IsDev:      isDev,
		ssrTempDir: "",
	}
}

func (r *Resolver) GetSSRBundlePath(ssrManifestPath string) string {
	if ssrManifestPath == "" {
		return ""
	}

	if r.AssetsFS == (embed.FS{}) {
		return filepath.Join(".bifrost", ssrManifestPath)
	}

	tempDir, err := r.extractSSRBundles()
	if err != nil {
		return ""
	}

	bundleName := filepath.Base(ssrManifestPath)
	return filepath.Join(tempDir, bundleName)
}

func (r *Resolver) extractSSRBundles() (string, error) {
	if r.ssrTempDir != "" {
		return r.ssrTempDir, nil
	}

	tempDir, err := os.MkdirTemp("", "bifrost-ssr-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir for SSR bundles: %w", err)
	}

	ssrDir := filepath.Join(".bifrost", "ssr")
	entries, err := r.AssetsFS.ReadDir(ssrDir)
	if err != nil {
		return tempDir, nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		data, err := r.AssetsFS.ReadFile(filepath.Join(ssrDir, entry.Name()))
		if err != nil {
			continue
		}

		if err := os.WriteFile(filepath.Join(tempDir, entry.Name()), data, 0644); err != nil {
			os.RemoveAll(tempDir)
			return "", fmt.Errorf("failed to write SSR bundle %s: %w", entry.Name(), err)
		}
	}

	r.ssrTempDir = tempDir
	return tempDir, nil
}

func (r *Resolver) Cleanup() {
	if r.ssrTempDir != "" {
		os.RemoveAll(r.ssrTempDir)
	}
}

var contentTypes = map[string]string{
	".css":   "text/css",
	".js":    "application/javascript",
	".json":  "application/json",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".svg":   "image/svg+xml",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".ttf":   "font/ttf",
	".eot":   "application/vnd.ms-fontobject",
	".ico":   "image/x-icon",
}

func GetContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ct, ok := contentTypes[ext]; ok {
		return ct
	}
	return "application/octet-stream"
}

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

		contentType := GetContentType(path)
		w.Header().Set("Content-Type", contentType)
		w.Write(data)
	})
}

func PublicHandler(assetsFS embed.FS, next http.Handler, isDev bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, "/")
		if path == "" {
			next.ServeHTTP(w, req)
			return
		}

		var exists bool
		if isDev {
			exists = PublicFileExistsInFS(path)
		} else {
			exists = PublicFileExistsInEmbed(assetsFS, path)
		}

		if !exists {
			next.ServeHTTP(w, req)
			return
		}

		if isDev {
			servePublicFromFS(w, req, path)
		} else {
			servePublicFromEmbed(assetsFS, w, req, path)
		}
	})
}

func PublicFileExistsInFS(path string) bool {
	fullPath := filepath.Join("public", path)
	info, err := os.Stat(fullPath)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func PublicFileExistsInEmbed(assetsFS embed.FS, path string) bool {
	embedPath := filepath.Join(".bifrost", "public", path)
	embedPath = filepath.ToSlash(embedPath)
	_, err := assetsFS.ReadFile(embedPath)
	return err == nil
}

func servePublicFromFS(w http.ResponseWriter, req *http.Request, path string) {
	fullPath := filepath.Join("public", path)

	info, err := os.Stat(fullPath)
	if err != nil {
		http.NotFound(w, req)
		return
	}

	if info.IsDir() {
		http.NotFound(w, req)
		return
	}

	file, err := os.Open(fullPath)
	if err != nil {
		http.NotFound(w, req)
		return
	}
	defer file.Close()

	http.ServeContent(w, req, info.Name(), info.ModTime(), file)
}

func servePublicFromEmbed(assetsFS embed.FS, w http.ResponseWriter, req *http.Request, path string) {
	embedPath := filepath.Join(".bifrost", "public", path)
	embedPath = filepath.ToSlash(embedPath)

	data, err := assetsFS.ReadFile(embedPath)
	if err != nil {
		http.NotFound(w, req)
		return
	}

	contentType := GetContentType(path)
	w.Header().Set("Content-Type", contentType)
	w.Write(data)
}

func ComponentImportPath(entryPath string, componentPath string) (string, error) {
	from := filepath.Dir(entryPath)
	rel, err := filepath.Rel(from, componentPath)
	if err != nil {
		return "", err
	}

	rel = strings.TrimSuffix(rel, filepath.Ext(rel))
	rel = filepath.ToSlash(rel)
	if strings.HasPrefix(rel, ".") {
		return rel, nil
	}

	return "./" + rel, nil
}
