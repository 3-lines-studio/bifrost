package web

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/andybalholm/brotli"
)

func assetServer(t *testing.T) (http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	writeAsset(t, dir, "app.wasm", "wasm-bytes")
	writeAsset(t, dir, "wasm_exec.js", "js-bytes")
	return FileServer(dir), dir
}

func writeAsset(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func serve(t *testing.T, handler http.Handler, request *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestFileServerBrotli(t *testing.T) {
	handler, _ := assetServer(t)
	request := httptest.NewRequest(http.MethodGet, "/app.wasm", nil)
	request.Header.Set("Accept-Encoding", "gzip, br")
	recorder := serve(t, handler, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("content encoding %q", got)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/wasm" {
		t.Fatalf("content type %q", got)
	}
	body, err := io.ReadAll(brotli.NewReader(bytes.NewReader(recorder.Body.Bytes())))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "wasm-bytes" {
		t.Fatalf("body %q", body)
	}
}

func TestFileServerGzip(t *testing.T) {
	handler, _ := assetServer(t)
	request := httptest.NewRequest(http.MethodGet, "/app.wasm", nil)
	request.Header.Set("Accept-Encoding", "gzip, deflate")
	recorder := serve(t, handler, request)
	if got := recorder.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("content encoding %q", got)
	}
	reader, err := gzip.NewReader(recorder.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "wasm-bytes" {
		t.Fatalf("body %q", body)
	}
}

func TestFileServerPlainWasm(t *testing.T) {
	handler, _ := assetServer(t)
	recorder := serve(t, handler, httptest.NewRequest(http.MethodGet, "/app.wasm", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("content encoding %q", got)
	}
	if recorder.Body.String() != "wasm-bytes" {
		t.Fatalf("body %q", recorder.Body.String())
	}
}

func TestFileServerNotModified(t *testing.T) {
	handler, _ := assetServer(t)
	first := serve(t, handler, httptest.NewRequest(http.MethodGet, "/app.wasm", nil))
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}
	request := httptest.NewRequest(http.MethodGet, "/app.wasm", nil)
	request.Header.Set("If-None-Match", etag)
	recorder := serve(t, handler, request)
	if recorder.Code != http.StatusNotModified {
		t.Fatalf("status %d", recorder.Code)
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("body %q", recorder.Body.String())
	}
}

func TestFileServerReloadsChangedFile(t *testing.T) {
	handler, dir := assetServer(t)
	first := serve(t, handler, httptest.NewRequest(http.MethodGet, "/app.wasm", nil))
	writeAsset(t, dir, "app.wasm", "wasm-bytes-two")
	second := serve(t, handler, httptest.NewRequest(http.MethodGet, "/app.wasm", nil))
	if second.Body.String() != "wasm-bytes-two" {
		t.Fatalf("body %q", second.Body.String())
	}
	if first.Header().Get("ETag") == second.Header().Get("ETag") {
		t.Fatal("ETag did not change")
	}
}

func TestFileServerCompressesAgainAfterChange(t *testing.T) {
	handler, dir := assetServer(t)
	request := httptest.NewRequest(http.MethodGet, "/app.wasm", nil)
	request.Header.Set("Accept-Encoding", "br")
	serve(t, handler, request)
	writeAsset(t, dir, "app.wasm", "wasm-bytes-two")
	recorder := serve(t, handler, request)
	if got := recorder.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("content encoding %q", got)
	}
	body, err := io.ReadAll(brotli.NewReader(bytes.NewReader(recorder.Body.Bytes())))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "wasm-bytes-two" {
		t.Fatalf("body %q", body)
	}
}

func TestFileServerExec(t *testing.T) {
	handler, _ := assetServer(t)
	recorder := serve(t, handler, httptest.NewRequest(http.MethodGet, "/wasm_exec.js", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "js-bytes" {
		t.Fatalf("status %d body %q", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("content type %q", got)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("exec content encoding %q", got)
	}
}

func TestFileServerHead(t *testing.T) {
	handler, _ := assetServer(t)
	recorder := serve(t, handler, httptest.NewRequest(http.MethodHead, "/app.wasm", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d", recorder.Code)
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("body %q", recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Length"); got != "10" {
		t.Fatalf("content length %q", got)
	}
}

func TestFileServerUnknownAndMissing(t *testing.T) {
	handler, dir := assetServer(t)
	recorder := serve(t, handler, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status %d", recorder.Code)
	}
	missing := FileServer(filepath.Join(dir, "missing"))
	recorder = serve(t, missing, httptest.NewRequest(http.MethodGet, "/app.wasm", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing status %d", recorder.Code)
	}
}

func TestAcceptedEncoding(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"", ""},
		{"gzip", gzipEncoding},
		{"br", brotliEncoding},
		{"gzip, br", brotliEncoding},
		{"gzip;q=1, br;q=0.5", gzipEncoding},
		{"br, gzip;q=0", brotliEncoding},
		{"br;q=0, gzip;q=0", ""},
		{"*", brotliEncoding},
		{"identity", ""},
		{"deflate, BR;q=0.8", brotliEncoding},
	}
	for _, testCase := range cases {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Accept-Encoding", testCase.header)
		if got := acceptedEncoding(request); got != testCase.want {
			t.Errorf("acceptedEncoding(%q) = %q, want %q", testCase.header, got, testCase.want)
		}
	}
}

func TestFileServerPrefersTheBuildSidecar(t *testing.T) {
	dir := t.TempDir()
	writeAsset(t, dir, "app.wasm", "wasm-bytes")
	writeAsset(t, dir, "wasm_exec.js", "js-bytes")
	writeAsset(t, dir, "app.wasm.br", "brotli-built")
	request := httptest.NewRequest(http.MethodGet, "/app.wasm", nil)
	request.Header.Set("Accept-Encoding", "br, gzip")
	response := serve(t, FileServer(dir), request)
	if got := response.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("encoding %q", got)
	}
	if got := response.Body.String(); got != "brotli-built" {
		t.Fatalf("body %q", got)
	}
}

func TestFileServerServesAnEmbeddedBuild(t *testing.T) {
	files := fstest.MapFS{
		"app.wasm":     &fstest.MapFile{Data: []byte("wasm-bytes")},
		"app.wasm.gz":  &fstest.MapFile{Data: []byte("gzip-built")},
		"wasm_exec.js": &fstest.MapFile{Data: []byte("js-bytes")},
	}
	request := httptest.NewRequest(http.MethodGet, "/app.wasm", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	response := serve(t, FileServerFS(files), request)
	if got := response.Body.String(); got != "gzip-built" {
		t.Fatalf("body %q", got)
	}
	if response := serve(t, FileServerFS(files), httptest.NewRequest(http.MethodGet, "/wasm_exec.js", nil)); response.Code != http.StatusOK {
		t.Fatalf("status %d", response.Code)
	}
	if response := serve(t, FileServerFS(files), httptest.NewRequest(http.MethodGet, "/missing", nil)); response.Code != http.StatusNotFound {
		t.Fatalf("status %d", response.Code)
	}
}

func TestReloadHandlerSendsTheProcessID(t *testing.T) {
	id := func() string {
		server := httptest.NewServer(ReloadHandler())
		defer server.Close()
		response, err := http.Get(server.URL + ReloadPath)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if got := response.Header.Get("Content-Type"); got != "text/event-stream" {
			t.Fatalf("content type %q", got)
		}
		line, err := bufio.NewReader(response.Body).ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		return line
	}
	first := id()
	if !strings.HasPrefix(first, "data: ") {
		t.Fatalf("first line %q", first)
	}
	if second := id(); second == first {
		t.Fatalf("two processes sent the same id %q", first)
	}
}
