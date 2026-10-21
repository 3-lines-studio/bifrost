package bifrost

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecoverPageWritesInternalServerError(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	func() {
		defer RecoverPage(recorder, request)
		panic("boom")
	}()
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestRecoverPageWritesDataError(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Accept", dataMediaType)
	func() {
		defer RecoverPage(recorder, request)
		panic("boom")
	}()
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if got := recorder.Header().Get("Content-Type"); got != dataMediaType {
		t.Fatalf("content type = %q, want %q", got, dataMediaType)
	}
}
