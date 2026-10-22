package bifrost

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func abortTestHandler(loadErr func(*http.Request) error, handled *bool) *serverPageHandler {
	h := &serverPageHandler{pattern: "/", load: func(r *http.Request) (any, error) { return nil, loadErr(r) }}
	h.hooks.errorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		*handled = true
		w.WriteHeader(http.StatusTeapot)
	}
	return h
}

func TestClientAbortDuringLoaderWrites499WithoutErrorHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	handled := false
	h := abortTestHandler(func(r *http.Request) error {
		cancel()
		return fmt.Errorf("query: %w", r.Context().Err())
	}, &handled)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx))
	if rec.Code != statusClientClosedRequest || rec.Body.Len() != 0 || handled {
		t.Fatalf("code=%d body=%q handled=%v", rec.Code, rec.Body.String(), handled)
	}
}

func TestCanceledErrorWithLiveRequestStillUsesErrorHandler(t *testing.T) {
	handled := false
	h := abortTestHandler(func(*http.Request) error { return context.Canceled }, &handled)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusTeapot || !handled {
		t.Fatalf("code=%d handled=%v", rec.Code, handled)
	}
}

func TestRequestDeadlineStillUsesErrorHandler(t *testing.T) {
	ctx, cancelDeadline := context.WithTimeout(context.Background(), 0)
	defer cancelDeadline()
	handled := false
	h := abortTestHandler(func(r *http.Request) error { return r.Context().Err() }, &handled)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx))
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) || !handled || rec.Code != http.StatusTeapot {
		t.Fatalf("code=%d handled=%v", rec.Code, handled)
	}
}
