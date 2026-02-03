package bifrost

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

func waitForSocket(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for bun socket at %s", path)
}

const scriptTagEndPattern = "</"

func htmlShell(bodyHTML string, props map[string]interface{}, scriptSrc string, title string, headHTML string, cssHref string, chunks []string) (string, error) {
	if scriptSrc == "" {
		return "", fmt.Errorf("missing script src")
	}

	if title == "" {
		title = "Bifrost"
	}

	if props == nil {
		props = map[string]interface{}{}
	}

	propsJSON, err := json.Marshal(props)
	if err != nil {
		return "", err
	}

	escapedProps := strings.ReplaceAll(string(propsJSON), scriptTagEndPattern, "<\\/")

	cssLink := ""
	if cssHref != "" {
		cssLink = "<link rel=\"stylesheet\" href=\"" + cssHref + "\" />"
	}

	head := "<meta charset=\"UTF-8\" /><meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\" />"
	headHTMLLower := strings.ToLower(headHTML)
	if !strings.Contains(headHTMLLower, "<title") {
		head += "<title>" + title + "</title>"
	}
	if headHTML != "" {
		head += headHTML
	}
	if cssLink != "" {
		head += cssLink
	}

	var buf bytes.Buffer
	if err := PageTemplate.Execute(&buf, map[string]interface{}{
		"Head":      template.HTML(head),
		"Body":      template.HTML(bodyHTML),
		"Props":     template.JS(escapedProps),
		"ScriptSrc": scriptSrc,
		"Chunks":    chunks,
	}); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func writeClientEntry(path string, componentImport string) error {
	if path == "" {
		return fmt.Errorf("missing entry path")
	}

	if componentImport == "" {
		return fmt.Errorf("missing component import")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := ClientEntryTemplate.Execute(&buf, map[string]string{
		"ComponentImport": componentImport,
	}); err != nil {
		return err
	}

	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func serveHTML(w http.ResponseWriter, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}

func loadManifest(path string) (*buildManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseManifest(data)
}

func parseManifest(data []byte) (*buildManifest, error) {
	var manifest buildManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func getAssetsFromManifest(man *buildManifest, entryName string) (scriptSrc, cssHref string, chunks []string) {
	if man != nil && man.Entries[entryName].Script != "" {
		entry := man.Entries[entryName]
		return entry.Script, entry.CSS, entry.Chunks
	}
	return "/dist/" + entryName + ".js", "/dist/" + entryName + ".css", nil
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

func addCacheBust(url string, value string) string {
	if value == "" {
		return url
	}

	separator := "?"
	if strings.Contains(url, "?") {
		separator = "&"
	}

	return url + separator + "v=" + value
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
}

func getContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ct, ok := contentTypes[ext]; ok {
		return ct
	}
	return "application/octet-stream"
}

func publicFileExistsInFS(path string) bool {
	fullPath := filepath.Join(PublicDir, path)
	info, err := os.Stat(fullPath)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func publicFileExistsInEmbed(assetsFS embed.FS, path string) bool {
	embedPath := filepath.Join(PublicDir, path)
	embedPath = filepath.ToSlash(embedPath)
	_, err := assetsFS.ReadFile(embedPath)
	return err == nil
}

func servePublicFromFS(w http.ResponseWriter, req *http.Request, path string) {
	fullPath := filepath.Join(PublicDir, path)

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
	embedPath := filepath.Join(PublicDir, path)
	embedPath = filepath.ToSlash(embedPath)

	data, err := assetsFS.ReadFile(embedPath)
	if err != nil {
		http.NotFound(w, req)
		return
	}

	contentType := getContentType(path)
	w.Header().Set("Content-Type", contentType)
	w.Write(data)
}

func watchDirs(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			log.Printf("Error accessing path %s: %v", path, err)
			return nil
		}

		if !d.IsDir() {
			return nil
		}

		if ShouldSkipDir(d.Name()) {
			return filepath.SkipDir
		}

		return watcher.Add(path)
	})
}

var skipDirs = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	".bifrost":     {},
	"public":       {},
}

func ShouldSkipDir(name string) bool {
	_, exists := skipDirs[name]
	return exists
}

func isWatchEvent(op fsnotify.Op) bool {
	return op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) != 0
}

func shouldAddWatchDir(event fsnotify.Event) bool {
	if event.Op&fsnotify.Create == 0 {
		return false
	}

	info, err := os.Stat(event.Name)
	if err != nil {
		return false
	}

	return info.IsDir() && !ShouldSkipDir(info.Name())
}

var rebuildExts = map[string]bool{
	".ts":  true,
	".tsx": true,
	".js":  true,
	".jsx": true,
	".css": true,
}

func ShouldRebuildForPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return rebuildExts[ext]
}
