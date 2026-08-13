package bifrost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type fakeRenderer struct {
	render func(context.Context, renderRequest, renderSink) error
	closed bool
}

func (r *fakeRenderer) Render(ctx context.Context, request renderRequest, sink renderSink) error {
	return r.render(ctx, request, sink)
}

func (r *fakeRenderer) Close(context.Context) error {
	r.closed = true
	return nil
}

func runtimeFixture(t *testing.T, render renderer) (*App, *runtimeState) {
	t.Helper()
	files, app, wireManifest := validManifestFixture(t)
	manifest, err := validateManifest(files, app.spec, app.specHash, wireManifest)
	if err != nil {
		t.Fatal(err)
	}
	state, err := compileRuntime(app, files, manifest, render)
	if err != nil {
		t.Fatal(err)
	}
	return app, state
}

func TestServerHandlerStreamsDocument(t *testing.T) {
	var received renderRequest
	render := &fakeRenderer{render: func(_ context.Context, request renderRequest, sink renderSink) error {
		received = request
		if err := sink.Head([]byte(`<title>Server</title>`)); err != nil {
			return err
		}
		if err := sink.Body([]byte(`<main>`)); err != nil {
			return err
		}
		return sink.Body([]byte(`hello</main>`))
	}}
	_, state := runtimeFixture(t, render)

	request := httptest.NewRequest(http.MethodGet, "/server", nil)
	response := httptest.NewRecorder()
	state.handlers["/server"].ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	body := response.Body.String()
	for _, expected := range []string{
		`<!doctype html>`,
		`<title>Server</title>`,
		`<main>hello</main>`,
		`id="__BIFROST_PROPS__"`,
		`src="/_bifrost/dist/server.js"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("response does not contain %q:\n%s", expected, body)
		}
	}
	if received.Entry != "ssr/server.js" {
		t.Fatalf("renderer entry = %q", received.Entry)
	}
	if string(received.Props) != `{}` {
		t.Fatalf("renderer props = %s", received.Props)
	}
}

func TestServerHandlerUsesRequestDocumentAttributes(t *testing.T) {
	files, app, wireManifest := validManifestFixture(t)
	for i := range app.routes {
		if app.routes[i].pattern == "/server" {
			app.routes[i].load = func(*http.Request) (any, error) {
				return PageData{Props: map[string]string{"name": "Don"}, Document: Document{Lang: "pt-BR", Class: "dark", Dir: "ltr"}}, nil
			}
		}
	}
	manifest, err := validateManifest(files, app.spec, app.specHash, wireManifest)
	if err != nil {
		t.Fatal(err)
	}
	render := &fakeRenderer{render: func(_ context.Context, _ renderRequest, sink renderSink) error {
		if err := sink.Head(nil); err != nil {
			return err
		}
		return sink.Body(nil)
	}}
	state, err := compileRuntime(app, files, manifest, render)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	state.handlers["/server"].ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/server", nil))
	for _, expected := range []string{`<html lang="pt-BR" class="dark" dir="ltr">`, `{"name":"Don"}`} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("response lacks %q: %s", expected, response.Body.String())
		}
	}
	if strings.Contains(response.Body.String(), "Document") {
		t.Fatal("document metadata leaked into hydration props")
	}
}

func TestServerHandlerRejectsInvalidDocumentAttributesBeforeRender(t *testing.T) {
	files, app, wireManifest := validManifestFixture(t)
	for i := range app.routes {
		if app.routes[i].pattern == "/server" {
			app.routes[i].load = func(*http.Request) (any, error) {
				return PageData{Document: Document{Lang: "bad language"}}, nil
			}
		}
	}
	manifest, err := validateManifest(files, app.spec, app.specHash, wireManifest)
	if err != nil {
		t.Fatal(err)
	}
	render := &fakeRenderer{render: func(context.Context, renderRequest, renderSink) error {
		t.Fatal("renderer called with invalid document attributes")
		return nil
	}}
	state, err := compileRuntime(app, files, manifest, render)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	state.handlers["/server"].ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/server", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestServerHandlerUsesSameEscapedPropsForRenderAndHydration(t *testing.T) {
	files, app, wireManifest := validManifestFixture(t)
	for i := range app.routes {
		if app.routes[i].pattern == "/server" {
			app.routes[i].load = func(*http.Request) (any, error) {
				return map[string]string{"unsafe": `</script><script>alert(1)</script>`}, nil
			}
		}
	}
	manifest, err := validateManifest(files, app.spec, app.specHash, wireManifest)
	if err != nil {
		t.Fatal(err)
	}
	var props json.RawMessage
	render := &fakeRenderer{render: func(_ context.Context, request renderRequest, sink renderSink) error {
		props = bytes.Clone(request.Props)
		if err := sink.Head(nil); err != nil {
			return err
		}
		return sink.Body(nil)
	}}
	state, err := compileRuntime(app, files, manifest, render)
	if err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	state.handlers["/server"].ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/server", nil))
	if strings.Contains(response.Body.String(), `</script><script>alert`) {
		t.Fatal("props escaped out of JSON script")
	}
	if !bytes.Contains(props, []byte(`\u003c/script\u003e`)) {
		t.Fatalf("renderer props were not escaped: %s", props)
	}
	if !bytes.Contains(response.Body.Bytes(), props) {
		t.Fatal("hydration does not contain renderer props bytes")
	}
}

func TestDevelopmentStaticHandlerUsesLiveRenderer(t *testing.T) {
	t.Setenv("BIFROST_DEV_DIR", t.TempDir())
	t.Setenv("BIFROST_VITE_PORT", "5173")
	var entry string
	render := &fakeRenderer{render: func(_ context.Context, request renderRequest, sink renderSink) error {
		entry = request.Entry
		if err := sink.Head(nil); err != nil {
			return err
		}
		return sink.Body([]byte("<h1>live static</h1>"))
	}}
	_, state := runtimeFixture(t, render)
	response := httptest.NewRecorder()
	state.handlers["/static"].ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/static", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "live static") || !strings.HasPrefix(entry, "/@fs/") {
		t.Fatalf("response = %d %q, entry = %q", response.Code, response.Body.String(), entry)
	}
}

func TestStaticAndClientHandlers(t *testing.T) {
	render := &fakeRenderer{render: func(_ context.Context, _ renderRequest, sink renderSink) error {
		if err := sink.Head(nil); err != nil {
			return err
		}
		return nil
	}}
	_, state := runtimeFixture(t, render)

	t.Run("static", func(t *testing.T) {
		response := httptest.NewRecorder()
		state.handlers["/static"].ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/static", nil))
		if response.Code != http.StatusOK || response.Body.String() != "<html>static</html>" {
			t.Fatalf("static response = %d %q", response.Code, response.Body.String())
		}
	})

	t.Run("client", func(t *testing.T) {
		response := httptest.NewRecorder()
		state.handlers["/client"].ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/client", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d", response.Code)
		}
		body := response.Body.String()
		if strings.Contains(body, "server bundle") || !strings.Contains(body, `src="/_bifrost/dist/client.js"`) {
			t.Fatalf("bad client document: %s", body)
		}
	})
}

func TestConcurrentServerRequestsKeepStateIsolated(t *testing.T) {
	files, app, wireManifest := validManifestFixture(t)
	for i := range app.routes {
		if app.routes[i].pattern == "/server" {
			app.routes[i].load = func(request *http.Request) (any, error) {
				value := request.URL.Query().Get("value")
				lang := "en"
				if value == "b" {
					lang = "es"
				}
				return PageData{Props: map[string]string{"value": value}, Document: Document{Lang: lang, Class: "request-" + value}}, nil
			}
		}
	}
	manifest, err := validateManifest(files, app.spec, app.specHash, wireManifest)
	if err != nil {
		t.Fatal(err)
	}
	render := &fakeRenderer{render: func(_ context.Context, request renderRequest, sink renderSink) error {
		if err := sink.Head(nil); err != nil {
			return err
		}
		return sink.Body([]byte(`<main>` + string(request.Props) + `</main>`))
	}}
	state, err := compileRuntime(app, files, manifest, render)
	if err != nil {
		t.Fatal(err)
	}

	var group sync.WaitGroup
	for _, value := range []string{"a", "b"} {
		group.Go(func() {
			for range 100 {
				response := httptest.NewRecorder()
				state.handlers["/server"].ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/server?value="+value, nil))
				lang := "en"
				if value == "b" {
					lang = "es"
				}
				for _, expected := range []string{`lang="` + lang + `" class="request-` + value + `"`, `{"value":"` + value + `"}`} {
					if !strings.Contains(response.Body.String(), expected) {
						t.Errorf("response for %q lacks %q: %s", value, expected, response.Body.String())
						return
					}
				}
			}
		})
	}
	group.Wait()
}

func TestRegisterComposesWithFallbackAndSharedMiddleware(t *testing.T) {
	render := &fakeRenderer{render: func(_ context.Context, _ renderRequest, sink renderSink) error {
		if err := sink.Head(nil); err != nil {
			return err
		}
		return sink.Body([]byte("<main>page</main>"))
	}}
	app, state := runtimeFixture(t, render)
	app.runtime = state
	mux := http.NewServeMux()
	if err := app.Register(mux); err != nil {
		t.Fatal(err)
	}
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("fallback"))
	}))
	handler := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("X-Shared-Middleware", "active")
		mux.ServeHTTP(w, request)
	})

	for _, test := range []struct {
		path string
		want string
	}{
		{path: "/server", want: "<main>page</main>"},
		{path: "/api/health", want: "fallback"},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if !strings.Contains(response.Body.String(), test.want) || response.Header().Get("X-Shared-Middleware") != "active" {
			t.Fatalf("%s response = %d %q, headers = %v", test.path, response.Code, response.Body.String(), response.Header())
		}
	}
}

func TestRendererOverloadReturnsServiceUnavailable(t *testing.T) {
	render := &fakeRenderer{render: func(context.Context, renderRequest, renderSink) error { return ErrRendererBusy }}
	_, state := runtimeFixture(t, render)
	response := httptest.NewRecorder()
	state.handlers["/server"].ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/server", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestServerErrorsBeforeAndAfterCommit(t *testing.T) {
	t.Run("before head", func(t *testing.T) {
		render := &fakeRenderer{render: func(context.Context, renderRequest, renderSink) error {
			return errors.New("render failed")
		}}
		_, state := runtimeFixture(t, render)
		response := httptest.NewRecorder()
		state.handlers["/server"].ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/server", nil))
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d", response.Code)
		}
	})

	t.Run("after head", func(t *testing.T) {
		render := &fakeRenderer{render: func(_ context.Context, _ renderRequest, sink renderSink) error {
			if err := sink.Head(nil); err != nil {
				return err
			}
			return errors.New("late failure")
		}}
		_, state := runtimeFixture(t, render)
		response := httptest.NewRecorder()
		state.handlers["/server"].ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/server", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d", response.Code)
		}
	})
}

func TestRedirectRejectsInvalidInput(t *testing.T) {
	for _, err := range []error{Redirect("", http.StatusFound), Redirect("/bad", http.StatusOK)} {
		var redirect redirectError
		if errors.As(err, &redirect) {
			t.Fatalf("invalid redirect became redirect outcome: %v", err)
		}
	}
}

func TestLoaderRedirectAndNotFound(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{name: "redirect", err: Redirect("/login", http.StatusTemporaryRedirect), status: http.StatusTemporaryRedirect},
		{name: "not found", err: NotFound(nil), status: http.StatusNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files, app, wireManifest := validManifestFixture(t)
			for i := range app.routes {
				if app.routes[i].pattern == "/server" {
					app.routes[i].load = func(*http.Request) (any, error) { return nil, test.err }
				}
			}
			manifest, err := validateManifest(files, app.spec, app.specHash, wireManifest)
			if err != nil {
				t.Fatal(err)
			}
			render := &fakeRenderer{render: func(context.Context, renderRequest, renderSink) error {
				t.Fatal("renderer called after loader error")
				return nil
			}}
			state, err := compileRuntime(app, files, manifest, render)
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			state.handlers["/server"].ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/server", nil))
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
		})
	}
}

func TestAssetHandlerUsesETag(t *testing.T) {
	render := &fakeRenderer{render: func(context.Context, renderRequest, renderSink) error { return nil }}
	_, state := runtimeFixture(t, render)
	handler := &assetHandler{assets: state.assets, files: state.files}

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/_bifrost/dist/client.js", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d", first.Code)
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}

	request := httptest.NewRequest(http.MethodGet, "/_bifrost/dist/client.js", nil)
	request.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, request)
	if second.Code != http.StatusNotModified || second.Body.Len() != 0 {
		t.Fatalf("conditional response = %d %q", second.Code, second.Body.String())
	}
}
