package http

import (
	"embed"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkCleanPath(b *testing.B) {
	paths := []string{"dist/app.js", "/dist/app.js", "dist/chunks/chunk-abc.js", "../etc/passwd"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cleanPath(paths[i%len(paths)])
	}
}

func BenchmarkContainsDotDot(b *testing.B) {
	paths := []string{"dist/app.js", "dist/../etc/passwd", "safe/path/file.css"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		containsDotDot(paths[i%len(paths)])
	}
}

func BenchmarkAssetHandler_ServeFromFS(b *testing.B) {
	origDir, _ := os.Getwd()
	tmpDir := b.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	bifrostDir := filepath.Join(tmpDir, ".bifrost", "dist")
	_ = os.MkdirAll(bifrostDir, 0755)
	_ = os.WriteFile(filepath.Join(bifrostDir, "app.js"), []byte("console.log('bench')"), 0644)

	handler := NewAssetHandler(embed.FS{}, true)
	req := httptest.NewRequest("GET", "/dist/app.js", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

func BenchmarkPublicHandler_ServeFromFS(b *testing.B) {
	origDir, _ := os.Getwd()
	tmpDir := b.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	publicDir := filepath.Join(tmpDir, "public")
	_ = os.MkdirAll(publicDir, 0755)
	_ = os.WriteFile(filepath.Join(publicDir, "favicon.ico"), []byte("icon-data"), 0644)

	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := NewPublicHandler(embed.FS{}, fallback, true)
	req := httptest.NewRequest("GET", "/favicon.ico", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

func BenchmarkSafeEmbedPath(b *testing.B) {
	paths := []string{"/pages/home.html", "/pages/routes/blog/hello/index.html", "/../../../etc/passwd"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		safeEmbedPath(paths[i%len(paths)])
	}
}
