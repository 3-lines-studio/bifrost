package bifrost

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAppWrapWithServeMux(t *testing.T) {
	skipIfNoBun(t)
	t.Setenv("BIFROST_DEV", "1")

	app := New(testFS, Page("/", "./example/components/hello.tsx"))
	defer func() { _ = app.Stop() }()

	api := http.NewServeMux()

	handler := app.Wrap(api)

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Errorf("Root path / returned 404, expected the page handler to be called")
	}

	req2 := httptest.NewRequest("GET", "/dist/test.js", nil)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
}

func TestAppHandlerNoRouter(t *testing.T) {
	skipIfNoBun(t)
	t.Setenv("BIFROST_DEV", "1")

	app := New(testFS, Page("/", "./test.tsx"))
	defer func() { _ = app.Stop() }()

	handler := app.Handler()

	if handler == nil {
		t.Error("Handler() returned nil handler")
	}

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Errorf("Root path / returned 404, expected the page handler to be called")
	}
}

func TestAppWrap(t *testing.T) {
	t.Setenv("BIFROST_DEV", "1")

	tests := []struct {
		name string
	}{
		{
			name: "App creates handler successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skipIfNoBun(t)
			app := New(testFS, Page("/", "./test.tsx"))
			defer func() { _ = app.Stop() }()

			api := http.NewServeMux()
			handler := app.Wrap(api)

			if handler == nil {
				t.Error("Wrap returned nil handler")
			}
		})
	}
}

func TestAppWrapNilPanics(t *testing.T) {
	skipIfNoBun(t)
	t.Setenv("BIFROST_DEV", "1")

	app := New(testFS, Page("/", "./test.tsx"))
	defer func() { _ = app.Stop() }()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Wrap(nil) should panic, but it didn't")
		}
	}()

	app.Wrap(nil)
}
