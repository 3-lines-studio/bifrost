//go:build !js

package client

import (
	"context"
	"encoding/json"
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
