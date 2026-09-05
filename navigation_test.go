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

func TestNavigationRunsLoaderAndReturnsPageData(t *testing.T) {
	render := &fakeRenderer{render: func(_ context.Context, request renderRequest, sink renderSink) error {
		if string(request.Props) != `{"slug":"hello"}` {
			t.Fatalf("props = %s", request.Props)
		}
		if err := sink.Head([]byte("<title>Hello</title>")); err != nil {
			return err
		}
		return sink.Body([]byte("<main>not sent</main>"))
	}}
	_, state := runtimeFixture(t, render)
	handler := state.handlers["/server"].(*serverPageHandler)
	handler.navigationView = "view"
	handler.navigationBuild = "build"
	handler.load = func(r *http.Request) (any, error) {
		return PageData{Props: map[string]string{"slug": r.URL.Query().Get("slug")}, Document: Document{Lang: "es", Class: "dark", Dir: "ltr"}, Status: http.StatusNotFound}, nil
	}
	request := httptest.NewRequest(http.MethodGet, "/server?slug=hello", nil)
	request.Header.Set("Accept", navigationMediaType)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || response.Header().Get("Content-Type") != navigationMediaType || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Vary") != "Accept" {
		t.Fatalf("response = %d %v", response.Code, response.Header())
	}
	var data navigationPage
	if err := json.Unmarshal(response.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if data.Build != "build" || data.View != "view" || data.Head != "<title>Hello</title>" || data.Document.Lang != "es" || data.Document.Class != "dark" || data.Document.Dir != "ltr" {
		t.Fatalf("data = %+v", data)
	}
	if strings.Contains(response.Body.String(), "not sent") {
		t.Fatal("navigation sent rendered body")
	}
}

func TestNavigationRenderErrorUsesBoundaryWithoutPartialResponse(t *testing.T) {
	calls := 0
	render := &fakeRenderer{render: func(_ context.Context, request renderRequest, sink renderSink) error {
		calls++
		if err := sink.Head([]byte("<title>Page</title>")); err != nil {
			return err
		}
		if err := sink.Body([]byte("<main>partial</main>")); err != nil {
			return err
		}
		if calls == 1 {
			return errors.New("private error")
		}
		return nil
	}}
	_, state := runtimeFixture(t, render)
	handler := state.handlers["/server"].(*serverPageHandler)
	handler.navigationView = "view"
	handler.navigationBuild = "build"
	handler.load = func(*http.Request) (any, error) {
		return PageData{ErrorFallbacks: 1}, nil
	}
	request := httptest.NewRequest(http.MethodGet, "/server", nil)
	request.Header.Set("Accept", navigationMediaType)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var data navigationPage
	if err := json.Unmarshal(response.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || response.Code != http.StatusInternalServerError || !strings.Contains(string(data.Props), "Internal Server Error") || strings.Contains(response.Body.String(), "private error") {
		t.Fatalf("response = %d %s, calls = %d", response.Code, response.Body.String(), calls)
	}
}

func TestNavigationLeavesOtherRequestsAsDocuments(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, accept := range []string{"text/html", navigationMediaType} {
			if enabled && accept == navigationMediaType {
				continue
			}
			t.Run(accept+"/"+map[bool]string{true: "enabled", false: "disabled"}[enabled], func(t *testing.T) {
				render := &fakeRenderer{render: func(_ context.Context, _ renderRequest, sink renderSink) error {
					return sink.Head([]byte("<title>Page</title>"))
				}}
				_, state := runtimeFixture(t, render)
				handler := state.handlers["/server"].(*serverPageHandler)
				if enabled {
					handler.navigationView = "view"
					handler.navigationBuild = "build"
					handler.shell = handler.shell.WithNavigation("build")
				}
				request := httptest.NewRequest(http.MethodGet, "/server", nil)
				request.Header.Set("Accept", accept)
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if response.Header().Get("Content-Type") != "text/html; charset=utf-8" {
					t.Fatalf("response = %v", response.Header())
				}
				if strings.Contains(response.Body.String(), `name="bifrost-build" content="build"`) != enabled {
					t.Fatalf("document = %s", response.Body.String())
				}
			})
		}
	}
}

func TestNavigationRedirectStaysHTTPRedirect(t *testing.T) {
	_, state := runtimeFixture(t, &fakeRenderer{})
	handler := state.handlers["/server"].(*serverPageHandler)
	handler.navigationView = "view"
	handler.load = func(*http.Request) (any, error) {
		return nil, Redirect("/login")
	}
	request := httptest.NewRequest(http.MethodGet, "/server", nil)
	request.Header.Set("Accept", navigationMediaType)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/login" {
		t.Fatalf("response = %d %v", response.Code, response.Header())
	}
}

func TestNavigationDeclarationIsImmutableAndChangesSpec(t *testing.T) {
	original := Server("/", "page.tsx", nil)
	enabled := original.WithNavigation()
	if original.navigation || !enabled.navigation || !enabled.WithNavigation().navigation {
		t.Fatal("navigation declaration is not immutable and idempotent")
	}
	_, originalHash, err := makeBuildSpec([]Route{original})
	if err != nil {
		t.Fatal(err)
	}
	spec, enabledHash, err := makeBuildSpec([]Route{enabled})
	if err != nil {
		t.Fatal(err)
	}
	if originalHash == enabledHash || !spec.Routes[0].Navigation {
		t.Fatal("navigation was omitted from the build spec")
	}
}

func TestNavigationRenderLimitsDoNotCommit(t *testing.T) {
	t.Run("head", func(t *testing.T) {
		response := httptest.NewRecorder()
		sink := &navigationRenderSink{writer: response, limits: Limits{MaxHeadBytes: 2}}
		if err := sink.Head([]byte("large")); err == nil {
			t.Fatal("oversized head accepted")
		}
		if sink.committed() || response.Body.Len() != 0 {
			t.Fatal("failed render committed")
		}
	})
	t.Run("body", func(t *testing.T) {
		response := httptest.NewRecorder()
		sink := &navigationRenderSink{writer: response, limits: Limits{MaxFrameBytes: 2}}
		if err := sink.Head(nil); err != nil {
			t.Fatal(err)
		}
		if err := sink.Body([]byte("large")); err == nil {
			t.Fatal("oversized body frame accepted")
		}
		if sink.committed() || response.Body.Len() != 0 {
			t.Fatal("failed render committed")
		}
	})
}
