package bifrost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPluginHooksCompileIntoRuntime(t *testing.T) {
	files, _, wireManifest := validManifestFixture(t)
	loadCalls := 0
	renderCalls := 0
	responseCalls := 0
	plugin := testPlugin{name: "hooks", register: func(registry *AppRegistry) error {
		if err := registry.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				w.Header().Set("X-AppPlugin", "yes")
				next.ServeHTTP(w, request)
			})
		}); err != nil {
			return err
		}
		if err := registry.OnLoad(func(context.Context, LoadEvent) { loadCalls++ }); err != nil {
			return err
		}
		if err := registry.OnRender(func(context.Context, RenderEvent) { renderCalls++ }); err != nil {
			return err
		}
		return registry.OnResponse(func(context.Context, ResponseEvent) { responseCalls++ })
	}}
	app, err := newApp(Config{
		SourceRoot: t.TempDir(),
		AppPlugins: []AppPlugin{plugin},
		Routes: []Route{
			Server("/server", "pages/server.tsx", func(*http.Request) (any, error) { return nil, nil }),
			Static("/static", "pages/static.tsx", nil),
			Client("/client", "pages/client.tsx"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	wireManifest.SpecHash = app.specHash
	manifest, err := validateManifest(files, app.spec, app.specHash, wireManifest)
	if err != nil {
		t.Fatal(err)
	}
	renderer := &fakeRenderer{render: func(_ context.Context, _ renderRequest, sink renderSink) error {
		if err := sink.Head(nil); err != nil {
			return err
		}
		return sink.Body([]byte("ok"))
	}}
	state, err := compileRuntime(app, files, manifest, renderer)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	state.handlers["/server"].ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/server", nil))
	if response.Header().Get("X-AppPlugin") != "yes" {
		t.Fatal("middleware did not run")
	}
	if loadCalls != 1 || renderCalls != 1 || responseCalls != 1 {
		t.Fatalf("hook calls = load:%d render:%d response:%d", loadCalls, renderCalls, responseCalls)
	}
}
