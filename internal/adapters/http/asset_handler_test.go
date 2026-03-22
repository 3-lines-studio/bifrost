package http

import (
	"embed"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCleanPath(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		wantOK bool
	}{
		{"normal path", "dist/app.js", "dist/app.js", true},
		{"leading slash", "/dist/app.js", "dist/app.js", true},
		{"double slash", "dist//app.js", "dist/app.js", true},
		{"dot-dot traversal", "../../../etc/passwd", "", false},
		{"mid-path traversal", "dist/../../etc/passwd", "", false},
		{"encoded traversal via backslash", "dist\\..\\..\\etc\\passwd", "", false},
		{"trailing dot-dot", "dist/..", "", false},
		{"just dot-dot", "..", "", false},
		{"empty", "", "", false},
		{"just slash", "/", "", false},
		{"just dot", ".", "", false},
		{"nested safe path", "dist/chunks/chunk-abc.js", "dist/chunks/chunk-abc.js", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := cleanPath(tt.input)
			if ok != tt.wantOK {
				t.Errorf("cleanPath(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("cleanPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAssetHandler_TraversalBlocked(t *testing.T) {
	handler := NewAssetHandler(embed.FS{}, true)

	traversalPaths := []string{
		"/../../etc/passwd",
		"/../.bifrost/manifest.json",
		"/dist/../../secret.txt",
		"/..%2f..%2fetc/passwd",
	}

	for _, p := range traversalPaths {
		t.Run(p, func(t *testing.T) {
			req := httptest.NewRequest("GET", p, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != http.StatusNotFound {
				t.Errorf("expected 404 for traversal path %q, got %d", p, w.Code)
			}
		})
	}
}

func chdirTemp(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(origDir); err != nil {
			t.Logf("warning: failed to restore working directory: %v", err)
		}
	})
	return tmpDir
}

func TestAssetHandler_ServesValidDevFile(t *testing.T) {
	tmpDir := chdirTemp(t)

	bifrostDir := filepath.Join(tmpDir, ".bifrost", "dist")
	if err := os.MkdirAll(bifrostDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bifrostDir, "app.js"), []byte("console.log('hi')"), 0644); err != nil {
		t.Fatal(err)
	}

	handler := NewAssetHandler(embed.FS{}, true)
	req := httptest.NewRequest("GET", "/dist/app.js", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "console.log('hi')" {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestAssetHandler_DevTraversalCannotEscapeBifrost(t *testing.T) {
	tmpDir := chdirTemp(t)

	if err := os.WriteFile(filepath.Join(tmpDir, "secret.txt"), []byte("TOP SECRET"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, ".bifrost"), 0755); err != nil {
		t.Fatal(err)
	}

	handler := NewAssetHandler(embed.FS{}, true)
	req := httptest.NewRequest("GET", "/../secret.txt", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for traversal, got %d", w.Code)
	}
}

func TestPublicHandler_TraversalBlocked(t *testing.T) {
	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	handler := NewPublicHandler(embed.FS{}, fallback, true)

	traversalPaths := []string{
		"/../../etc/passwd",
		"/../secret.txt",
	}

	for _, p := range traversalPaths {
		t.Run(p, func(t *testing.T) {
			req := httptest.NewRequest("GET", p, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != http.StatusTeapot {
				t.Errorf("expected fallback (418) for traversal path %q, got %d", p, w.Code)
			}
		})
	}
}

func TestPublicHandler_ServesValidDevFile(t *testing.T) {
	tmpDir := chdirTemp(t)

	publicDir := filepath.Join(tmpDir, "public")
	if err := os.MkdirAll(publicDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(publicDir, "favicon.ico"), []byte("icon"), 0644); err != nil {
		t.Fatal(err)
	}

	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	handler := NewPublicHandler(embed.FS{}, fallback, true)
	req := httptest.NewRequest("GET", "/favicon.ico", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSafeEmbedPath(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		wantOK bool
	}{
		{"normal", "/pages/home.html", ".bifrost/pages/home.html", true},
		{"traversal", "/../../../etc/passwd", "", false},
		{"dot-dot", "..", "", false},
		{"empty", "", "", false},
		{"nested", "/pages/routes/blog/hello/index.html", ".bifrost/pages/routes/blog/hello/index.html", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := safeEmbedPath(tt.input)
			if ok != tt.wantOK {
				t.Errorf("safeEmbedPath(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("safeEmbedPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
