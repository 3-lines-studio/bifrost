package bifrost

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
)

type testPlugin struct {
	name     string
	register func(*AppRegistry) error
}

func (p testPlugin) Name() string { return p.name }

func (p testPlugin) Register(registry *AppRegistry) error {
	if p.register == nil {
		return nil
	}
	return p.register(registry)
}

func TestNewBuildsNormalizedModel(t *testing.T) {
	load := func(*http.Request) (any, error) { return map[string]any{"ok": true}, nil }
	generate := func(context.Context) ([]StaticPage, error) {
		return []StaticPage{{Path: "/blog/first"}}, nil
	}

	app, err := newApp(Config{
		SourceRoot: t.TempDir(),
		Routes: []Route{
			Client("/app", "./pages/app.tsx"),
			Server("/{$}", "pages/home.tsx", load),
			Static("/blog/{slug}", "pages/blog.tsx", generate),
			Server("/shared", "pages/home.tsx", nil),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got, want := len(app.routes), 4; got != want {
		t.Fatalf("route count = %d, want %d", got, want)
	}
	if got, want := app.routes[0].view, "pages/app.tsx"; got != want {
		t.Fatalf("normalized view = %q, want %q", got, want)
	}
	if got, want := len(app.spec.Routes), 4; got != want {
		t.Fatalf("spec route count = %d, want %d", got, want)
	}
	if app.specHash == "" {
		t.Fatal("empty build spec hash")
	}

	patterns := make([]string, len(app.spec.Routes))
	for i, route := range app.spec.Routes {
		patterns[i] = route.Pattern
	}
	if !slices.IsSorted(patterns) {
		t.Fatalf("build routes are not sorted: %v", patterns)
	}
}

func TestBuildSpecIsIndependentOfDeclarationOrder(t *testing.T) {
	routesA := []Route{
		Server("/b", "pages/shared.tsx", nil),
		Client("/a", "pages/app.tsx"),
	}
	routesB := []Route{routesA[1], routesA[0]}

	appA, err := newApp(Config{SourceRoot: t.TempDir(), Routes: routesA})
	if err != nil {
		t.Fatal(err)
	}
	appB, err := newApp(Config{SourceRoot: t.TempDir(), Routes: routesB})
	if err != nil {
		t.Fatal(err)
	}
	if appA.specHash != appB.specHash {
		t.Fatalf("hash differs by declaration order: %s != %s", appA.specHash, appB.specHash)
	}
}

func TestAppPluginRegistersTypedServerExtensions(t *testing.T) {
	var retained *AppRegistry
	plugin := testPlugin{
		name: "example",
		register: func(registry *AppRegistry) error {
			retained = registry
			if err := registry.AddRoutes(Client("/plugin", "pages/plugin.tsx")); err != nil {
				return err
			}
			if err := registry.Use(func(next http.Handler) http.Handler { return next }); err != nil {
				return err
			}
			if err := registry.OnLoad(func(context.Context, LoadEvent) {}); err != nil {
				return err
			}
			return registry.OnQueue(func(context.Context, QueueEvent) {})
		},
	}

	app, err := newApp(Config{SourceRoot: t.TempDir(), AppPlugins: []AppPlugin{plugin}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(app.routes), 1; got != want {
		t.Fatalf("plugin routes = %d, want %d", got, want)
	}
	if got, want := len(app.hooks.middleware), 1; got != want {
		t.Fatalf("middleware count = %d, want %d", got, want)
	}
	if got, want := len(app.hooks.loadHooks), 1; got != want {
		t.Fatalf("load hook count = %d, want %d", got, want)
	}
	if got, want := len(app.hooks.queueHooks), 1; got != want {
		t.Fatalf("queue hook count = %d, want %d", got, want)
	}
	if err := retained.AddRoutes(Client("/late", "pages/late.tsx")); !errors.Is(err, errRegistrySealed) {
		t.Fatalf("late registry use error = %v, want registry sealed", err)
	}
}

func TestNewRejectsInvalidDeclarations(t *testing.T) {
	dynamicGenerator := func(context.Context) ([]StaticPage, error) { return nil, nil }

	tests := []struct {
		name   string
		routes []Route
		match  string
	}{
		{name: "zero route", routes: []Route{{}}, match: "was not created"},
		{name: "empty pattern", routes: []Route{Client("", "pages/app.tsx")}, match: "pattern is empty"},
		{name: "method in pattern", routes: []Route{Client("GET /app", "pages/app.tsx")}, match: "must start with /"},
		{name: "query", routes: []Route{Client("/app?q=x", "pages/app.tsx")}, match: "query or fragment"},
		{name: "reserved prefix", routes: []Route{Client("/_bifrost/debug", "pages/app.tsx")}, match: "reserved Bifrost asset prefix"},
		{name: "empty view", routes: []Route{Client("/app", "")}, match: "view path is empty"},
		{name: "absolute view", routes: []Route{Client("/app", "/tmp/app.tsx")}, match: "must be relative"},
		{name: "escaping view", routes: []Route{Client("/app", "../app.tsx")}, match: "escapes the source root"},
		{name: "backslash view", routes: []Route{Client("/app", `pages\\app.tsx`)}, match: "forward slashes"},
		{name: "unsupported view", routes: []Route{Client("/app", "pages/app.vue")}, match: "must use .tsx"},
		{name: "dynamic static without generator", routes: []Route{Static("/blog/{slug}", "pages/blog.tsx", nil)}, match: "requires a generator"},
		{name: "duplicate", routes: []Route{Client("/app", "pages/a.tsx"), Client("/app", "pages/b.tsx")}, match: "duplicate route pattern"},
		{name: "mux conflict", routes: []Route{Client("/{first}", "pages/a.tsx"), Static("/{second}", "pages/b.tsx", dynamicGenerator)}, match: "conflicts with pattern"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newApp(Config{SourceRoot: t.TempDir(), Routes: test.routes})
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("New error = %v, want match %q", err, test.match)
			}
		})
	}
}

func TestExactStaticRouteDoesNotRequireGenerator(t *testing.T) {
	for _, pattern := range []string{"/about", "/{$}", "/docs/{$}"} {
		t.Run(pattern, func(t *testing.T) {
			_, err := newApp(Config{
				SourceRoot: t.TempDir(),
				Routes:     []Route{Static(pattern, "pages/static.tsx", nil)},
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNewRejectsInvalidPlugins(t *testing.T) {
	tests := []struct {
		name    string
		plugins []AppPlugin
		match   string
	}{
		{name: "nil", plugins: []AppPlugin{nil}, match: "plugin 0 is nil"},
		{name: "empty name", plugins: []AppPlugin{testPlugin{}}, match: "empty name"},
		{name: "duplicate name", plugins: []AppPlugin{testPlugin{name: "x"}, testPlugin{name: "x"}}, match: "duplicate plugin name"},
		{name: "registration error", plugins: []AppPlugin{testPlugin{name: "x", register: func(*AppRegistry) error { return errors.New("bad") }}}, match: `register plugin "x": bad`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newApp(Config{SourceRoot: t.TempDir(), AppPlugins: test.plugins})
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("New error = %v, want match %q", err, test.match)
			}
		})
	}
}

func TestNewCopiesInputSlices(t *testing.T) {
	routes := []Route{Client("/a", "pages/a.tsx")}
	app, err := newApp(Config{SourceRoot: t.TempDir(), Routes: routes})
	if err != nil {
		t.Fatal(err)
	}
	routes[0] = Client("/changed", "pages/changed.tsx")
	if got, want := app.routes[0].pattern, "/a"; got != want {
		t.Fatalf("app route changed through input slice: got %q, want %q", got, want)
	}
}

func TestNewRequiresAssetsOutsideBuildPhase(t *testing.T) {
	_, err := New(Config{Routes: []Route{Client("/app", "pages/app.tsx")}})
	if err == nil || !strings.Contains(err.Error(), "Config.Assets is required") {
		t.Fatalf("New error = %v", err)
	}
}

func TestMustNewPanicsOnInvalidConfig(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustNew did not panic")
		}
	}()
	MustNew(Config{Routes: []Route{Client("bad", "pages/app.tsx")}})
}
