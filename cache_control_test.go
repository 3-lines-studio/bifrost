package bifrost

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCacheControlHandlerSetsHeaderUnlessHandlerDoes(t *testing.T) {
	recorder := httptest.NewRecorder()
	handler := &cacheControlHandler{
		value: "public, max-age=60",
		next: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if w.Header().Get("Cache-Control") != "public, max-age=60" {
				t.Fatalf("header = %q", w.Header().Get("Cache-Control"))
			}
			w.Header().Set("Cache-Control", "private")
		}),
	}
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/", nil))
	if got := recorder.Header().Get("Cache-Control"); got != "private" {
		t.Fatalf("final header = %q", got)
	}
}
