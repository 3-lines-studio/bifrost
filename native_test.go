package bifrost

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func newNativeApp(t *testing.T, config Config) *App {
	t.Helper()
	app, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func serveNative(t *testing.T, app *App, request *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	return recorder
}

func staticNativeRender(page NativePage) NativeRender {
	return func(context.Context, json.RawMessage) (NativePage, error) {
		return page, nil
	}
}

func TestNativeRendersDocument(t *testing.T) {
	render := func(_ context.Context, props json.RawMessage) (NativePage, error) {
		var data struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal(props, &data); err != nil {
			return NativePage{}, err
		}
		return NativePage{
			Title:       data.Title,
			Description: "A <page>",
			Canonical:   "https://example.com/",
			Body:        []byte("<h1>Hello</h1>"),
		}, nil
	}
	load := func(*http.Request) (any, error) {
		return PageData{
			Props:    map[string]string{"title": "Home & Away"},
			Document: Document{Lang: "pt-BR", Class: "dark", Dir: "ltr"},
			Status:   http.StatusCreated,
		}, nil
	}
	app := newNativeApp(t, Config{Routes: []Route{Native("/{$}", render, load)}})

	response := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusCreated {
		t.Fatalf("status %d, want %d", response.Code, http.StatusCreated)
	}
	if got := response.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("content type %q", got)
	}
	if got := response.Header().Get("Cache-Control"); got != defaultDocumentCacheControl {
		t.Errorf("cache control %q", got)
	}
	body := response.Body.String()
	wants := []string{
		`<!doctype html><html lang="pt-BR" dir="ltr" class="dark">`,
		`<title>Home &amp; Away</title>`,
		`<meta name="description" content="A &lt;page&gt;">`,
		`<link rel="canonical" href="https://example.com/">`,
		`<h1>Hello</h1>`,
		`</body></html>`,
	}
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("body misses %q:\n%s", want, body)
		}
	}
}

func TestNativeDefaults(t *testing.T) {
	var received string
	render := func(_ context.Context, props json.RawMessage) (NativePage, error) {
		received = string(props)
		return NativePage{Body: []byte("ok")}, nil
	}
	app := newNativeApp(t, Config{Routes: []Route{Native("/", render, nil)}})

	response := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status %d", response.Code)
	}
	if received != "{}" {
		t.Errorf("props %q, want {}", received)
	}
	if !strings.Contains(response.Body.String(), `<html lang="en">`) {
		t.Errorf("body misses the default language: %s", response.Body.String())
	}
}

func TestNativeCacheControl(t *testing.T) {
	route := Native("/", staticNativeRender(NativePage{Body: []byte("ok")}), nil).WithCache("public, s-maxage=60")
	app := newNativeApp(t, Config{Routes: []Route{route}})

	response := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := response.Header().Get("Cache-Control"); got != "public, s-maxage=60" {
		t.Errorf("cache control %q", got)
	}
}

func TestNativeHead(t *testing.T) {
	app := newNativeApp(t, Config{Routes: []Route{Native("/", staticNativeRender(NativePage{Title: "T", Body: []byte("body")}), nil)}})

	response := serveNative(t, app, httptest.NewRequest(http.MethodHead, "/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status %d", response.Code)
	}
	if response.Body.Len() != 0 {
		t.Errorf("HEAD wrote %d bytes", response.Body.Len())
	}
	if got := response.Header().Get("Content-Length"); got == "" || got == "0" {
		t.Errorf("content length %q", got)
	}
}

func TestNativeMethodNotAllowed(t *testing.T) {
	app := newNativeApp(t, Config{Routes: []Route{Native("/", staticNativeRender(NativePage{Body: []byte("ok")}), nil)}})

	response := serveNative(t, app, httptest.NewRequest(http.MethodPost, "/", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Errorf("status %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func TestNativeLoadErrors(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		load := func(*http.Request) (any, error) { return nil, NotFound() }
		app := newNativeApp(t, Config{Routes: []Route{Native("/", staticNativeRender(NativePage{}), load)}})

		response := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/", nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("status %d", response.Code)
		}
	})

	t.Run("redirect", func(t *testing.T) {
		load := func(*http.Request) (any, error) { return nil, Redirect("/next", http.StatusMovedPermanently) }
		app := newNativeApp(t, Config{Routes: []Route{Native("/", staticNativeRender(NativePage{}), load)}})

		response := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/", nil))
		if response.Code != http.StatusMovedPermanently {
			t.Errorf("status %d", response.Code)
		}
		if got := response.Header().Get("Location"); got != "/next" {
			t.Errorf("location %q", got)
		}
	})

	t.Run("error handler", func(t *testing.T) {
		plugin := nativePlugin{
			handleErrors: func(w http.ResponseWriter, _ *http.Request, err error) {
				w.WriteHeader(http.StatusTeapot)
				_, _ = w.Write([]byte(err.Error()))
			},
		}
		render := func(context.Context, json.RawMessage) (NativePage, error) {
			return NativePage{}, statusError{status: http.StatusBadRequest}
		}
		app := newNativeApp(t, Config{Routes: []Route{Native("/", render, nil)}, AppPlugins: []AppPlugin{plugin}})

		response := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/", nil))
		if response.Code != http.StatusTeapot {
			t.Errorf("status %d", response.Code)
		}
		if response.Body.String() != "Bad Request" {
			t.Errorf("body %q", response.Body.String())
		}
	})
}

func TestNativeLimits(t *testing.T) {
	t.Run("body", func(t *testing.T) {
		app := newNativeApp(t, Config{
			Routes: []Route{Native("/", staticNativeRender(NativePage{Body: []byte(strings.Repeat("x", 64))}), nil)},
			Limits: Limits{MaxFrameBytes: 16},
		})

		response := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/", nil))
		if response.Code != http.StatusInternalServerError {
			t.Errorf("status %d", response.Code)
		}
	})

	t.Run("head", func(t *testing.T) {
		page := NativePage{Title: strings.Repeat("x", 64), Body: []byte("ok")}
		app := newNativeApp(t, Config{
			Routes: []Route{Native("/", staticNativeRender(page), nil)},
			Limits: Limits{MaxHeadBytes: 16},
		})

		response := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/", nil))
		if response.Code != http.StatusInternalServerError {
			t.Errorf("status %d", response.Code)
		}
	})

	t.Run("props", func(t *testing.T) {
		load := func(*http.Request) (any, error) {
			return map[string]string{"value": strings.Repeat("x", 64)}, nil
		}
		app := newNativeApp(t, Config{
			Routes: []Route{Native("/", staticNativeRender(NativePage{Body: []byte("ok")}), load)},
			Limits: Limits{MaxPropsBytes: 16},
		})

		response := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/", nil))
		if response.Code != http.StatusInternalServerError {
			t.Errorf("status %d", response.Code)
		}
	})
}

func TestNativeValidation(t *testing.T) {
	cases := []struct {
		name   string
		config Config
		want   string
	}{
		{
			name:   "nil render",
			config: Config{Routes: []Route{Native("/", nil, nil)}},
			want:   "no render function",
		},
		{
			name:   "server without assets",
			config: Config{Routes: []Route{Server("/", "pages/home.tsx", nil)}},
			want:   "Config.Assets is required",
		},
		{
			name:   "empty app",
			config: Config{},
			want:   "Config.Assets is required",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := New(testCase.config)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error %v, want %q", err, testCase.want)
			}
		})
	}
}

func TestNativeWithAssets(t *testing.T) {
	files, fixture, wireManifest := validManifestFixture(t)
	app, err := newApp(Config{
		SourceRoot: fixture.sourceRoot,
		Routes: []Route{
			Server("/server", "pages/server.tsx", nil),
			Static("/static", "pages/static.tsx", nil),
			Client("/client", "pages/client.tsx"),
			Native("/native", staticNativeRender(NativePage{Title: "Native", Body: []byte("<p>native</p>")}), nil),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := validateManifest(files, app.spec, app.specHash, wireManifest)
	if err != nil {
		t.Fatal(err)
	}
	render := &fakeRenderer{render: func(_ context.Context, _ renderRequest, sink renderSink) error {
		if err := sink.Head([]byte("<title>server</title>")); err != nil {
			return err
		}
		return sink.Body([]byte("server"))
	}}
	state, err := compileRuntime(app, files, manifest, render)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.addNativeHandlers(state); err != nil {
		t.Fatal(err)
	}
	app.runtime = state

	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/native", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "<p>native</p>") {
		t.Fatalf("native: %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/server", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "server") {
		t.Fatalf("server: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestNativeHooks(t *testing.T) {
	var loads int
	var responses []ResponseEvent
	plugin := nativePlugin{
		middleware: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				w.Header().Set("X-Native", "1")
				next.ServeHTTP(w, request)
			})
		},
		onLoad: func(context.Context, LoadEvent) { loads++ },
		onResponse: func(_ context.Context, event ResponseEvent) {
			responses = append(responses, event)
		},
	}
	load := func(*http.Request) (any, error) { return map[string]string{"a": "b"}, nil }
	app := newNativeApp(t, Config{Routes: []Route{Native("/", staticNativeRender(NativePage{Body: []byte("ok")}), load)}, AppPlugins: []AppPlugin{plugin}})

	response := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := response.Header().Get("X-Native"); got != "1" {
		t.Errorf("middleware header %q", got)
	}
	if loads != 1 {
		t.Errorf("load hooks %d", loads)
	}
	if len(responses) != 1 || responses[0].Status != http.StatusOK || responses[0].Pattern != "/" {
		t.Errorf("responses %+v", responses)
	}
	if err := app.Ready(context.Background()); err != nil {
		t.Errorf("ready: %v", err)
	}
	if err := app.Close(context.Background()); err != nil {
		t.Errorf("close: %v", err)
	}
}

type nativePlugin struct {
	middleware   Middleware
	handleErrors ErrorHandler
	onLoad       LoadHook
	onResponse   ResponseHook
}

func (nativePlugin) Name() string { return "native-test" }

func (p nativePlugin) Register(registry *AppRegistry) error {
	if p.middleware != nil {
		if err := registry.Use(p.middleware); err != nil {
			return err
		}
	}
	if p.handleErrors != nil {
		if err := registry.HandleErrors(p.handleErrors); err != nil {
			return err
		}
	}
	if p.onLoad != nil {
		if err := registry.OnLoad(p.onLoad); err != nil {
			return err
		}
	}
	if p.onResponse != nil {
		if err := registry.OnResponse(p.onResponse); err != nil {
			return err
		}
	}
	return nil
}

func TestNativeErrorPage(t *testing.T) {
	load := func(*http.Request) (any, error) {
		return nil, errors.New("the store is down")
	}
	errorRender := func(_ context.Context, err error) (NativePage, error) {
		return NativePage{Title: "Oops", Body: []byte("<h1>" + err.Error() + "</h1>")}, nil
	}
	app := newNativeApp(t, Config{Routes: []Route{
		Native("/{$}", staticNativeRender(NativePage{}), load).
			WithError(errorRender).
			WithCache("public, s-maxage=60"),
	}})
	response := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", response.Code)
	}
	if body := response.Body.String(); !strings.Contains(body, "<h1>the store is down</h1>") {
		t.Errorf("body %s", body)
	}
	if got := response.Header().Get("Cache-Control"); got != defaultDocumentCacheControl {
		t.Errorf("error page is cached: %q", got)
	}
}

func TestNativeErrorPageKeepsStatus(t *testing.T) {
	load := func(*http.Request) (any, error) {
		return nil, NotFound(errors.New("no such note"))
	}
	errorRender := func(_ context.Context, err error) (NativePage, error) {
		return NativePage{Title: "Missing", Body: []byte("<h1>Missing</h1>")}, nil
	}
	app := newNativeApp(t, Config{Routes: []Route{Native("/{$}", staticNativeRender(NativePage{}), load).WithError(errorRender)}})
	response := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "<h1>Missing</h1>") {
		t.Errorf("body %s", response.Body.String())
	}
}

func TestNativeErrorPageSkipsRedirectsAndHooks(t *testing.T) {
	errorRender := func(context.Context, error) (NativePage, error) {
		return NativePage{Title: "Wrong", Body: []byte("<h1>Wrong</h1>")}, nil
	}
	redirect := newNativeApp(t, Config{Routes: []Route{
		Native("/{$}", staticNativeRender(NativePage{}), func(*http.Request) (any, error) {
			return nil, Redirect("/login")
		}).WithError(errorRender),
	}})
	response := serveNative(t, redirect, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/login" {
		t.Fatalf("redirect answered %d %q", response.Code, response.Header().Get("Location"))
	}

	handled := newNativeApp(t, Config{
		Routes: []Route{
			Native("/{$}", staticNativeRender(NativePage{}), func(*http.Request) (any, error) {
				return nil, errors.New("boom")
			}).WithError(errorRender),
		},
		AppPlugins: []AppPlugin{nativePlugin{handleErrors: func(w http.ResponseWriter, _ *http.Request, _ error) {
			w.WriteHeader(http.StatusTeapot)
		}}},
	})
	response = serveNative(t, handled, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusTeapot {
		t.Fatalf("hook answered %d", response.Code)
	}
}

func TestNativeErrorPageRenderFailure(t *testing.T) {
	app := newNativeApp(t, Config{Routes: []Route{
		Native("/{$}", staticNativeRender(NativePage{}), func(*http.Request) (any, error) {
			return nil, errors.New("boom")
		}).WithError(func(context.Context, error) (NativePage, error) {
			return NativePage{}, errors.New("no error page")
		}),
	}})
	response := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", response.Code)
	}
}

func TestNativeErrorPageHead(t *testing.T) {
	app := newNativeApp(t, Config{Routes: []Route{
		Native("/{$}", staticNativeRender(NativePage{}), func(*http.Request) (any, error) {
			return nil, errors.New("boom")
		}).WithError(func(context.Context, error) (NativePage, error) {
			return NativePage{Body: []byte("<h1>Oops</h1>")}, nil
		}),
	}})
	response := serveNative(t, app, httptest.NewRequest(http.MethodHead, "/", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", response.Code)
	}
	if response.Body.Len() != 0 {
		t.Fatalf("head wrote %q", response.Body.String())
	}
	if response.Header().Get("Content-Length") == "" {
		t.Fatal("head has no content length")
	}
}

func TestWithErrorRequiresNativeRoute(t *testing.T) {
	load := func(*http.Request) (any, error) { return nil, nil }
	_, err := New(Config{Routes: []Route{Server("/", "page.tsx", load).WithError(func(context.Context, error) (NativePage, error) {
		return NativePage{}, nil
	})}})
	if err == nil || !strings.Contains(err.Error(), "native route") {
		t.Fatalf("error %v", err)
	}
}

func TestNativePageHeaders(t *testing.T) {
	load := func(*http.Request) (any, error) {
		return PageData{
			Status: http.StatusCreated,
			Headers: http.Header{
				"Set-Cookie":        {"session=abc; Path=/; HttpOnly"},
				"Vary":              {"Accept-Language"},
				"Content-Length":    {"1"},
				"Content-Type":      {"text/plain"},
				"X-Answer":          {"yes"},
				"Transfer-Encoding": {"chunked"},
			},
		}, nil
	}
	app := newNativeApp(t, Config{Routes: []Route{Native("/{$}", staticNativeRender(NativePage{Title: "Home"}), load)}})
	recorder := serveNative(t, app, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status %d", recorder.Code)
	}
	if got := recorder.Header().Get("Set-Cookie"); got != "session=abc; Path=/; HttpOnly" {
		t.Errorf("cookie %q", got)
	}
	if got := recorder.Header().Get("X-Answer"); got != "yes" {
		t.Errorf("app header %q", got)
	}
	if got := recorder.Header().Get("Vary"); got != "Accept-Language" {
		t.Errorf("vary %q", got)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("content type %q", got)
	}
	if got := recorder.Header().Get("Content-Length"); got != strconv.Itoa(recorder.Body.Len()) {
		t.Errorf("content length %q for %d bytes", got, recorder.Body.Len())
	}
	if got := recorder.Header().Get("Transfer-Encoding"); got != "" {
		t.Errorf("transfer encoding %q", got)
	}
}

func TestServerPageHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	applyPageHeaders(recorder, &PageData{
		Headers: http.Header{
			"Set-Cookie":     {"session=abc"},
			"Content-Type":   {"text/plain"},
			"X-From-The-App": {"yes"},
		},
	})
	if got := recorder.Header().Get("Set-Cookie"); got != "session=abc" {
		t.Errorf("cookie %q", got)
	}
	if got := recorder.Header().Get("X-From-The-App"); got != "yes" {
		t.Errorf("header %q", got)
	}
	if got := recorder.Header().Get("Content-Type"); got != "" {
		t.Errorf("a loader wrote the content type %q", got)
	}
	recorder = httptest.NewRecorder()
	applyPageHeaders(recorder, map[string]any{"headers": "no"})
	if got := recorder.Header().Get("Set-Cookie"); got != "" {
		t.Errorf("props wrote a cookie %q", got)
	}
}
