package bifrost

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

func compressedRequest(target string, header http.Header) *http.Request {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.RequestURI = ""
	if header != nil {
		request.Header = header
	}
	return request
}

func gzipAccepted() http.Header {
	header := http.Header{}
	header.Set("Accept-Encoding", "gzip, deflate, br")
	return header
}

func TestCompressShrinksDocuments(t *testing.T) {
	body := strings.Repeat("<!doctype html><p>hola</p>", 200)
	handler := Compress(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, body)
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	response, err := http.DefaultClient.Do(compressedRequest(server.URL, gzipAccepted()))
	if err != nil {
		t.Fatal(err)
	}
	if got := response.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("content encoding = %q, want gzip", got)
	}
	if got := response.Header.Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
		t.Fatalf("vary = %q", got)
	}
	// The framework drops the original length; the Go server may set one for the
	// compressed body it buffered, and either way it cannot be the old size.
	if got := response.Header.Get("Content-Length"); got != "" {
		size, err := strconv.Atoi(got)
		if err != nil || size >= len(body) {
			t.Fatalf("content length = %q, want fewer than %d bytes", got, len(body))
		}
	}
	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != body {
		t.Fatalf("body does not round trip: %d bytes", len(data))
	}
}

func TestCompressLeavesWhatDoesNotShrink(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		encoding    string
		cache       string
		length      string
		accept      string
		status      int
	}{
		{name: "font", contentType: "font/woff2", accept: "gzip"},
		{name: "image", contentType: "image/png", accept: "gzip"},
		{name: "video", contentType: "video/mp4", accept: "gzip"},
		{name: "event stream", contentType: "text/event-stream", accept: "gzip"},
		{name: "already encoded", contentType: "text/html", encoding: "br", accept: "gzip"},
		{name: "no transform", contentType: "text/html", cache: "public, no-transform", accept: "gzip"},
		{name: "tiny", contentType: "text/html", length: "200", accept: "gzip"},
		{name: "client without gzip", contentType: "text/html", accept: "br"},
		{name: "no content", contentType: "text/html", accept: "gzip", status: http.StatusNoContent},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			handler := Compress(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if testCase.contentType != "" {
					w.Header().Set("Content-Type", testCase.contentType)
				}
				if testCase.encoding != "" {
					w.Header().Set("Content-Encoding", testCase.encoding)
				}
				if testCase.cache != "" {
					w.Header().Set("Cache-Control", testCase.cache)
				}
				if testCase.length != "" {
					w.Header().Set("Content-Length", testCase.length)
				}
				status := testCase.status
				if status == 0 {
					status = http.StatusOK
				}
				w.WriteHeader(status)
				if status != http.StatusNoContent {
					_, _ = io.WriteString(w, strings.Repeat("hola", 500))
				}
			}))
			header := http.Header{}
			if testCase.accept != "" {
				header.Set("Accept-Encoding", testCase.accept)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, compressedRequest("/", header))
			if got := recorder.Header().Get("Content-Encoding"); got != testCase.encoding {
				t.Fatalf("content encoding = %q, want %q", got, testCase.encoding)
			}
			if testCase.length != "" {
				if got := recorder.Header().Get("Content-Length"); got != testCase.length {
					t.Fatalf("content length = %q, want %q", got, testCase.length)
				}
			}
			if testCase.name == "client without gzip" && !strings.Contains(recorder.Body.String(), "hola") {
				t.Fatalf("body was altered: %q", recorder.Body.String())
			}
		})
	}
}

func TestCompressKeepsStreamsFlowing(t *testing.T) {
	handler := Compress(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("the middleware hides the flusher")
		}
		for range 3 {
			_, _ = io.WriteString(w, "frame\n")
			flusher.Flush()
		}
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, compressedRequest("/", gzipAccepted()))

	if got := recorder.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("content encoding = %q", got)
	}
	reader, err := gzip.NewReader(recorder.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "frame\nframe\nframe\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestCompressReadsQuality(t *testing.T) {
	cases := map[string]bool{
		"gzip":             true,
		"gzip;q=0.5":       true,
		"gzip;q=0":         false,
		"br, gzip;q=0.001": true,
		"deflate, br":      false,
		"identity":         false,
		"GZIP":             true,
		"br;q=1, gzip;q=0": false,
	}
	for header, want := range cases {
		if got := acceptsGzip(header); got != want {
			t.Fatalf("acceptsGzip(%q) = %t, want %t", header, got, want)
		}
	}
}

func TestCompressNilHandler(t *testing.T) {
	recorder := httptest.NewRecorder()
	Compress(nil).ServeHTTP(recorder, compressedRequest("/", gzipAccepted()))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func TestHTMLPageIsCompressed(t *testing.T) {
	templates := fstest.MapFS{aboutTemplates: {Data: []byte(`<h1>{{.Title}}</h1>`)}}
	app := newHTMLApp(t, templates, HTML("/about", HTMLPage{
		Page: aboutTemplates,
		Load: func(*http.Request) (any, error) { return aboutData{Title: strings.Repeat("titulo ", 400)}, nil },
	}))
	state := htmlHandler(t, app, templates)
	server := httptest.NewServer(Compress(state.handlers["/about"]))
	defer server.Close()

	response, err := http.DefaultClient.Do(compressedRequest(server.URL, gzipAccepted()))
	if err != nil {
		t.Fatal(err)
	}
	if got := response.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("content encoding = %q, want gzip", got)
	}
	if got := response.Header.Get("Cache-Control"); got != defaultDocumentCacheControl {
		t.Fatalf("cache control = %q", got)
	}
	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	// The page is long enough to be worth compressing, which is what makes the
	// document shorter than the markup above.
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "<h1>titulo") {
		t.Fatalf("body lacks the page: %d bytes", len(data))
	}
}

func TestCompressLeavesCloudflareRequestsAlone(t *testing.T) {
	handler := Compress(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(strings.Repeat("<p>hola</p>", 400)))
	}))

	request := httptest.NewRequest(http.MethodGet, "/about", nil)
	request.Header.Set("Accept-Encoding", "br, gzip")
	request.Header.Set("CF-Ray", "8f3c1a2b9d4e5f60-EZE")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if encoding := response.Header().Get("Content-Encoding"); encoding != "" {
		t.Fatalf("content encoding = %q, the edge compresses this one", encoding)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", contentType)
	}
}
