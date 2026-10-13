package bifrost

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDataReturnsLoaderPropsWithoutRendering(t *testing.T) {
	rendered := false
	render := &fakeRenderer{render: func(_ context.Context, _ renderRequest, _ renderSink) error {
		rendered = true
		return errors.New("the renderer must not run for a data request")
	}}
	_, state := runtimeFixture(t, render)
	handler := state.handlers["/server"].(*serverPageHandler)
	handler.load = func(r *http.Request) (any, error) {
		return PageData{
			Props:    map[string]string{"slug": r.URL.Query().Get("slug")},
			Document: Document{Lang: "es", Class: "dark", Dir: "ltr"},
			Status:   http.StatusAccepted,
		}, nil
	}

	request := httptest.NewRequest(http.MethodGet, "/server?slug=hello", nil)
	request.Header.Set("Accept", dataMediaType)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if rendered {
		t.Fatal("data request rendered the page")
	}
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	if got := response.Header().Get("Content-Type"); got != dataMediaType {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := response.Header().Get("Vary"); got != "Accept" {
		t.Fatalf("Vary = %q", got)
	}
	var page dataPage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Pattern != "/server" || page.Status != http.StatusAccepted {
		t.Fatalf("page = %+v", page)
	}
	if string(page.Props) != `{"slug":"hello"}` {
		t.Fatalf("props = %s", page.Props)
	}
	if page.Document.Lang != "es" || page.Document.Class != "dark" || page.Document.Dir != "ltr" {
		t.Fatalf("document = %+v", page.Document)
	}
}

func TestDocumentRequestStillRenders(t *testing.T) {
	rendered := false
	render := &fakeRenderer{render: func(_ context.Context, _ renderRequest, sink renderSink) error {
		rendered = true
		if err := sink.Head([]byte(`<title>Server</title>`)); err != nil {
			return err
		}
		return sink.Body([]byte(`<main>hello</main>`))
	}}
	_, state := runtimeFixture(t, render)

	request := httptest.NewRequest(http.MethodGet, "/server", nil)
	response := httptest.NewRecorder()
	state.handlers["/server"].ServeHTTP(response, request)

	if !rendered {
		t.Fatal("document request did not render the page")
	}
	if !strings.Contains(response.Body.String(), `<main>hello</main>`) {
		t.Fatalf("body = %s", response.Body)
	}
	if got := response.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
}

func TestDataReportsLoaderErrorsAsJSON(t *testing.T) {
	render := &fakeRenderer{render: func(_ context.Context, _ renderRequest, _ renderSink) error {
		return errors.New("the renderer must not run for a data request")
	}}
	_, state := runtimeFixture(t, render)
	handler := state.handlers["/server"].(*serverPageHandler)

	for name, testCase := range map[string]struct {
		err    error
		status int
	}{
		"not found":     {err: NotFound(), status: http.StatusNotFound},
		"forbidden":     {err: Status(http.StatusForbidden, nil), status: http.StatusForbidden},
		"unknown":       {err: errors.New("private detail"), status: http.StatusInternalServerError},
		"renderer busy": {err: ErrRendererBusy, status: http.StatusServiceUnavailable},
	} {
		handler.load = func(*http.Request) (any, error) { return nil, testCase.err }
		request := httptest.NewRequest(http.MethodGet, "/server", nil)
		request.Header.Set("Accept", dataMediaType)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		if response.Code != testCase.status {
			t.Fatalf("%s: status = %d", name, response.Code)
		}
		if got := response.Header().Get("Content-Type"); got != dataMediaType {
			t.Fatalf("%s: Content-Type = %q", name, got)
		}
		var failure dataError
		if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if failure.Status != testCase.status {
			t.Fatalf("%s: failure = %+v", name, failure)
		}
		if strings.Contains(response.Body.String(), "private detail") {
			t.Fatalf("%s: the response leaked the loader error", name)
		}
	}
}

func TestDataKeepsRedirects(t *testing.T) {
	render := &fakeRenderer{render: func(_ context.Context, _ renderRequest, _ renderSink) error {
		return errors.New("the renderer must not run for a data request")
	}}
	_, state := runtimeFixture(t, render)
	handler := state.handlers["/server"].(*serverPageHandler)
	handler.load = func(*http.Request) (any, error) {
		return nil, Redirect("/login", http.StatusSeeOther)
	}

	request := httptest.NewRequest(http.MethodGet, "/server", nil)
	request.Header.Set("Accept", dataMediaType)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("Location"); got != "/login" {
		t.Fatalf("Location = %q", got)
	}
	if response.Body.Len() != 0 {
		t.Fatalf("redirect wrote a body: %s", response.Body)
	}
}

func TestDataTellsTheClientHowDeepTheErrorViewsGo(t *testing.T) {
	// A page can throw while it renders, and a client that renders the view
	// itself is the one that has to fall into error.tsx. Which one it lands in
	// is a number the server counts, so the answer has to travel with the props.
	render := &fakeRenderer{render: func(_ context.Context, _ renderRequest, _ renderSink) error {
		return errors.New("the renderer must not run for a data request")
	}}
	_, state := runtimeFixture(t, render)
	handler := state.handlers["/server"].(*serverPageHandler)
	handler.load = func(*http.Request) (any, error) {
		return PageData{Props: map[string]any{}, ErrorFallbacks: 2}, nil
	}

	request := httptest.NewRequest(http.MethodGet, "/server", nil)
	request.Header.Set("Accept", dataMediaType)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	var page dataPage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.ErrorFallbacks != 2 {
		t.Fatalf("errorFallbacks = %d, want 2", page.ErrorFallbacks)
	}
}
