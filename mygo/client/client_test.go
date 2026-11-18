//go:build !js

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /notes", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]string{{"title": "one"}})
	})
	mux.HandleFunc("POST /echo", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Errorf("authorization %q", got)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("body: %v", err)
		}
		if body["hello"] != "world" {
			t.Errorf("body %v", body)
		}
		w.Header().Set("X-Answer", "yes")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestGetJSON(t *testing.T) {
	server := newServer(t)
	notes, err := GetJSON[[]map[string]string](context.Background(), server.URL+"/notes", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || notes[0]["title"] != "one" {
		t.Fatalf("notes %v", notes)
	}
}

func TestGetStatusError(t *testing.T) {
	server := newServer(t)
	_, err := Get(context.Background(), server.URL+"/missing", nil)
	if err == nil {
		t.Fatal("missing route answered")
	}
}

func TestSendJSON(t *testing.T) {
	server := newServer(t)
	answer, err := SendJSON[map[string]string](context.Background(), http.MethodPost, server.URL+"/echo", map[string]string{"Authorization": "Bearer token"}, map[string]string{"hello": "world"})
	if err != nil {
		t.Fatal(err)
	}
	if answer["status"] != "ok" {
		t.Fatalf("answer %v", answer)
	}
}

func TestCanceled(t *testing.T) {
	server := newServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Get(ctx, server.URL+"/notes", nil)
	if err != context.Canceled {
		t.Fatalf("error %v", err)
	}
}

func TestBlobSendsTheRange(t *testing.T) {
	body := []byte("0123456789")
	blob := BytesBlob(body).Slice(2, 6)
	var sent []int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if string(data) != "2345" {
			t.Errorf("body %q", data)
		}
		if r.ContentLength != 4 {
			t.Errorf("content length %d", r.ContentLength)
		}
	}))
	defer server.Close()

	_, err := Do(context.Background(), Request{
		Method:   http.MethodPut,
		URL:      server.URL,
		Blob:     blob,
		OnUpload: func(n int64) { sent = append(sent, n) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sent) == 0 || sent[len(sent)-1] != 4 {
		t.Fatalf("progress %v", sent)
	}
	for i := 1; i < len(sent); i++ {
		if sent[i] < sent[i-1] {
			t.Fatalf("progress went back: %v", sent)
		}
	}
}

func TestBlobSliceOutside(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a slice past the end did not panic")
		}
	}()
	BytesBlob([]byte("0123")).Slice(0, 5)
}

func TestBodyAndBlob(t *testing.T) {
	_, err := Do(context.Background(), Request{Method: http.MethodPost, URL: "http://example.invalid", Body: []byte("x"), Blob: BytesBlob([]byte("y"))})
	if err == nil {
		t.Fatal("a request with a body and a blob was sent")
	}
}

func TestReaderBlob(t *testing.T) {
	file := bytes.NewReader([]byte("abcdefgh"))
	blob := ReaderBlob(file, 8).Slice(4, 8)
	if blob.Size() != 4 {
		t.Fatalf("size %d", blob.Size())
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if string(data) != "efgh" {
			t.Errorf("body %q", data)
		}
	}))
	defer server.Close()
	if _, err := Do(context.Background(), Request{Method: http.MethodPut, URL: server.URL, Blob: blob}); err != nil {
		t.Fatal(err)
	}
}
