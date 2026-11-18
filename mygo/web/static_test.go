package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"
)

func assets() fs.FS {
	return fstest.MapFS{
		"favicon.svg":  &fstest.MapFile{Data: []byte("<svg></svg>")},
		"css/site.css": &fstest.MapFile{Data: []byte("body{}")},
	}
}

func TestStaticServesWithACacheHeader(t *testing.T) {
	handler := Static(assets(), 24*time.Hour)
	request := httptest.NewRequest("GET", "/favicon.svg", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d", recorder.Code)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "public, max-age=86400" {
		t.Fatalf("cache %q", got)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("nosniff %q", got)
	}
	if recorder.Body.String() != "<svg></svg>" {
		t.Fatalf("body %q", recorder.Body.String())
	}
}

func TestStaticServesFilesOfFolders(t *testing.T) {
	handler := Static(assets(), time.Hour)
	request := httptest.NewRequest("GET", "/css/site.css", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "body{}" {
		t.Fatalf("status %d body %q", recorder.Code, recorder.Body.String())
	}
}

func TestStaticMissing(t *testing.T) {
	handler := Static(assets(), time.Hour)
	request := httptest.NewRequest("GET", "/nope.png", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status %d", recorder.Code)
	}
}

func TestStaticWithoutACacheHeader(t *testing.T) {
	handler := Static(assets(), 0)
	request := httptest.NewRequest("GET", "/favicon.svg", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if got := recorder.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("cache %q", got)
	}
}

func TestStaticListsNoFolder(t *testing.T) {
	handler := Static(assets(), time.Hour)
	for _, path := range []string{"/", "/css/"} {
		request := httptest.NewRequest("GET", path, nil)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s answered %d", path, recorder.Code)
		}
	}
}
