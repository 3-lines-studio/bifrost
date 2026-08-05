package app

import (
	"bytes"
	"context"
	"embed"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/3-lines-studio/bifrost/internal/adapters/runtime"
	"github.com/3-lines-studio/bifrost/internal/core"
)

var testFS embed.FS

func setSSRTempDir(t *testing.T, host *runtime.Host, tempDir string) {
	t.Helper()
	field := reflect.ValueOf(host).Elem().FieldByName("ssrTempDir")
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().SetString(tempDir)
}

func bunAvailable() bool {
	_, err := exec.LookPath("bun")
	return err == nil
}

func skipIfNoBun(t *testing.T) {
	if !bunAvailable() {
		t.Skip("bun not available, skipping integration test")
	}
}

func mustNew(t *testing.T, routes ...core.Route) *App {
	t.Helper()
	a, err := New(testFS, routes...)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return a
}

func TestNewCreatesApp(t *testing.T) {
	skipIfNoBun(t)
	t.Setenv("BIFROST_DEV", "1")

	a := mustNew(t)
	defer func() { _ = a.Stop() }()

	if a == nil {
		t.Error("New() returned nil app")
	}
}

func TestAddRoutesRejectsMixedModesForSharedComponent(t *testing.T) {
	a := &App{pageConfigs: make(map[string]*core.PageConfig)}
	if err := a.addRoutes([]core.Route{core.Page("/ssr", "./shared.tsx")}); err != nil {
		t.Fatalf("addRoutes() error: %v", err)
	}

	err := a.addRoutes([]core.Route{
		core.Page("/new", "./new.tsx"),
		core.Page("/client", "./shared.tsx", core.WithClient()),
	})
	if err == nil {
		t.Fatal("expected mixed modes for one component to return an error")
	}
	if len(a.routes) != 1 {
		t.Fatalf("failed route batch changed app routes: got %d routes", len(a.routes))
	}
}

func TestNewRejectsMixedModesForSharedComponent(t *testing.T) {
	t.Setenv("BIFROST_EXPORT", "1")

	a, err := New(testFS,
		core.Page("/ssr", "./shared.tsx"),
		core.Page("/client", "./shared.tsx", core.WithClient()),
	)
	if err == nil {
		t.Fatal("expected mixed modes for one component to return an error")
	}
	if a != nil {
		t.Fatal("expected nil app on route validation error")
	}
}

func TestAddRoutesRejectsDifferentClientDocumentAttrsForSharedComponent(t *testing.T) {
	a := &App{pageConfigs: make(map[string]*core.PageConfig)}
	err := a.addRoutes([]core.Route{
		core.Page("/first", "./shared.tsx", core.WithClient(), core.WithHTMLLang("en")),
		core.Page("/second", "./shared.tsx", core.WithClient(), core.WithHTMLLang("fr")),
	})
	if err == nil || !strings.Contains(err.Error(), "different HTML attributes") {
		t.Fatalf("addRoutes() error = %v, want client attribute conflict", err)
	}
	if len(a.routes) != 0 {
		t.Fatal("failed route batch changed app routes")
	}
}

func TestNewRejectsConflictingPageOptions(t *testing.T) {
	t.Setenv("BIFROST_EXPORT", "1")

	a, err := New(testFS, core.Page("/", "./page.tsx", core.WithClient(), core.WithStatic()))
	if err == nil {
		t.Fatal("expected conflicting page options to return an error")
	}
	if a != nil {
		t.Fatal("expected nil app on page validation error")
	}
}

func TestNewWithOptionsRejectsNilOption(t *testing.T) {
	t.Setenv("BIFROST_EXPORT", "1")

	a, err := NewWithOptions(testFS, []core.ConfigOption{nil})
	if err == nil {
		t.Fatal("expected nil config option to return an error")
	}
	if a != nil {
		t.Fatal("expected nil app on config validation error")
	}
}

func TestAddRoutesRejectsDuplicatePatterns(t *testing.T) {
	a := &App{pageConfigs: make(map[string]*core.PageConfig)}
	err := a.addRoutes([]core.Route{
		core.Page("/same", "./first.tsx"),
		core.Page("/same", "./second.tsx"),
	})
	if err == nil {
		t.Fatal("expected duplicate pattern to return an error")
	}
	if len(a.routes) != 0 {
		t.Fatal("failed route batch changed app routes")
	}
}

func TestAddRoutesRejectsEntryNameCollisions(t *testing.T) {
	a := &App{pageConfigs: make(map[string]*core.PageConfig)}
	err := a.addRoutes([]core.Route{
		core.Page("/nested", "./pages/foo/bar.tsx"),
		core.Page("/flat", "./pages/foo-bar.tsx"),
	})
	if err == nil {
		t.Fatal("expected build entry collision to return an error")
	}
	if len(a.routes) != 0 {
		t.Fatal("failed route batch changed app routes")
	}
}

func TestHandleRejectsMissingProductionManifestEntry(t *testing.T) {
	a := &App{
		pageConfigs: make(map[string]*core.PageConfig),
		manifest:    &core.Manifest{Entries: map[string]core.ManifestEntry{}},
	}

	err := a.Handle(core.Page("/new", "./new.tsx"))
	if err == nil || !strings.Contains(err.Error(), "missing manifest entry") {
		t.Fatalf("Handle() error = %v, want missing manifest entry", err)
	}
	if len(a.routes) != 0 {
		t.Fatal("failed Handle changed app routes")
	}
}

func TestHandleBeforeWrap(t *testing.T) {
	skipIfNoBun(t)
	t.Setenv("BIFROST_DEV", "1")

	a := mustNew(t)
	defer func() { _ = a.Stop() }()

	if err := a.Handle(core.Page("/", "./example/components/hello.tsx")); err != nil {
		t.Fatalf("Handle() error: %v", err)
	}

	api := http.NewServeMux()
	handler := a.Wrap(api)

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Errorf("root / returned 404 after Handle before Wrap")
	}
}

func TestHandleAfterWrapReturnsError(t *testing.T) {
	skipIfNoBun(t)
	t.Setenv("BIFROST_DEV", "1")

	a := mustNew(t, core.Page("/", "./test.tsx"))
	defer func() { _ = a.Stop() }()

	_ = a.Wrap(http.NewServeMux())

	if err := a.Handle(core.Page("/other", "./other.tsx")); err == nil {
		t.Error("Handle after Wrap should return an error")
	}
}

func TestStrictProductionRequirements(t *testing.T) {
	t.Setenv("BIFROST_DEV", "")

	t.Run("production without assets FS returns error", func(t *testing.T) {
		a, err := New(testFS)
		if err == nil {
			if a != nil {
				_ = a.Stop()
			}
			t.Fatal("expected production setup error")
		}
		if a != nil {
			t.Fatal("expected nil app on setup error")
		}
		if !strings.Contains(err.Error(), "embed.FS is required") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestGetStaticPathUsesExtractedSSRBundleInProduction(t *testing.T) {
	t.Setenv("BIFROST_DEV", "")

	a := &App{
		isDev: false,
		host:  &runtime.Host{},
		manifest: &core.Manifest{
			Entries: map[string]core.ManifestEntry{
				"pages-home-entry": {SSR: "/ssr/pages-home-entry-ssr.js", Mode: "ssr"},
			},
		},
	}
	config := core.PageConfig{
		ComponentPath: "./pages/home.tsx",
		Mode:          core.ModeSSR,
	}

	if got := a.getStaticPath(config); got != "/ssr/pages-home-entry-ssr.js" {
		t.Fatalf("getStaticPath() without staged bundles = %q", got)
	}

	tempDir := t.TempDir()
	a.host = &runtime.Host{}
	setSSRTempDir(t, a.host, tempDir)

	got := a.getStaticPath(config)
	want := filepath.Join(tempDir, "ssr", "pages-home-entry-ssr.js")
	if got != want {
		t.Fatalf("getStaticPath() with staged bundles = %q, want %q", got, want)
	}
}

func TestGetSSBundlePathUsesExtractedSSRBundleInProduction(t *testing.T) {
	t.Setenv("BIFROST_DEV", "")

	a := &App{
		host: &runtime.Host{},
		manifest: &core.Manifest{
			Entries: map[string]core.ManifestEntry{
				"pages-home-entry": {SSR: "/ssr/pages-home-entry-ssr.js", Mode: "ssr"},
			},
		},
	}

	if got := a.getSSBundlePath("pages-home-entry"); got != "/ssr/pages-home-entry-ssr.js" {
		t.Fatalf("getSSBundlePath() without staged bundles = %q", got)
	}

	tempDir := t.TempDir()
	setSSRTempDir(t, a.host, tempDir)

	got := a.getSSBundlePath("pages-home-entry")
	want := filepath.Join(tempDir, "ssr", "pages-home-entry-ssr.js")
	if got != want {
		t.Fatalf("getSSBundlePath() with staged bundles = %q, want %q", got, want)
	}
}

func TestAppWrapWithServeMux(t *testing.T) {
	skipIfNoBun(t)
	t.Setenv("BIFROST_DEV", "1")

	a := mustNew(t, core.Page("/", "./example/components/hello.tsx"))
	defer func() { _ = a.Stop() }()

	api := http.NewServeMux()

	handler := a.Wrap(api)

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

	a := mustNew(t, core.Page("/", "./test.tsx"))
	defer func() { _ = a.Stop() }()

	handler := a.Handler()

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
			a := mustNew(t, core.Page("/", "./test.tsx"))
			defer func() { _ = a.Stop() }()

			api := http.NewServeMux()
			handler := a.Wrap(api)

			if handler == nil {
				t.Error("Wrap returned nil handler")
			}
		})
	}
}

func TestAppWrapNilPanics(t *testing.T) {
	skipIfNoBun(t)
	t.Setenv("BIFROST_DEV", "1")

	a := mustNew(t, core.Page("/", "./test.tsx"))
	defer func() { _ = a.Stop() }()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Wrap(nil) should panic, but it didn't")
		}
	}()

	a.Wrap(nil)
}

func TestPageModeTypes(t *testing.T) {
	t.Run("SSR page has correct mode", func(t *testing.T) {
		skipIfNoBun(t)
		t.Setenv("BIFROST_DEV", "1")

		a := mustNew(t, core.Page("/test", "./test.tsx", core.WithLoader(func(*http.Request) (any, error) {
			return map[string]any{}, nil
		})))
		defer func() { _ = a.Stop() }()

		config := a.pageConfigs["./test.tsx"]
		if config == nil {
			t.Fatal("Config not stored")
		}
		if config.Mode != core.ModeSSR {
			t.Errorf("Expected ModeSSR, got %v", config.Mode)
		}
	})

	t.Run("Client page has correct mode", func(t *testing.T) {
		skipIfNoBun(t)
		t.Setenv("BIFROST_DEV", "1")

		a := mustNew(t, core.Page("/test", "./test.tsx", core.WithClient()))
		defer func() { _ = a.Stop() }()

		config := a.pageConfigs["./test.tsx"]
		if config == nil {
			t.Fatal("Config not stored")
		}
		if config.Mode != core.ModeClientOnly {
			t.Errorf("Expected ModeClientOnly, got %v", config.Mode)
		}
	})

	t.Run("Static page has correct mode", func(t *testing.T) {
		skipIfNoBun(t)
		t.Setenv("BIFROST_DEV", "1")

		a := mustNew(t, core.Page("/test", "./test.tsx", core.WithStatic()))
		defer func() { _ = a.Stop() }()

		config := a.pageConfigs["./test.tsx"]
		if config == nil {
			t.Fatal("Config not stored")
		}
		if config.Mode != core.ModeStaticPrerender {
			t.Errorf("Expected ModeStaticPrerender, got %v", config.Mode)
		}
	})
}

func TestWithStaticData(t *testing.T) {
	skipIfNoBun(t)
	t.Setenv("BIFROST_DEV", "1")

	loader := func(ctx context.Context) ([]core.StaticPathData, error) {
		return []core.StaticPathData{
			{Path: "/test", Props: map[string]any{"key": "value"}},
		}, nil
	}

	route := core.Page("/blog", "./blog.tsx", core.WithStaticData(loader))

	a := mustNew(t, route)
	defer func() { _ = a.Stop() }()

	config := a.pageConfigs["./blog.tsx"]
	if config == nil {
		t.Fatal("Config not stored")
	}
	if config.StaticDataLoader == nil {
		t.Error("StaticDataLoader not set in config")
	}
	if config.Mode != core.ModeStaticPrerender {
		t.Errorf("Expected ModeStaticPrerender, got %v", config.Mode)
	}
}

func TestDevModeWithStaticData(t *testing.T) {
	skipIfNoBun(t)
	t.Setenv("BIFROST_DEV", "1")

	loader := func(ctx context.Context) ([]core.StaticPathData, error) {
		return []core.StaticPathData{
			{
				Path: "/blog/hello",
				Props: map[string]any{
					"title": "Hello Post",
					"body":  "Hello content",
				},
			},
			{
				Path: "/blog/world",
				Props: map[string]any{
					"title": "World Post",
					"body":  "World content",
				},
			},
		}, nil
	}

	route := core.Page("/blog", "./blog.tsx", core.WithStaticData(loader))

	a := mustNew(t, route)
	defer func() { _ = a.Stop() }()

	config := a.pageConfigs["./blog.tsx"]
	if config == nil {
		t.Fatal("Config not stored")
	}

	if config.StaticDataLoader == nil {
		t.Error("StaticDataLoader not set")
	}
}

func TestWrapPrintsRouteTableWhenForced(t *testing.T) {
	skipIfNoBun(t)
	t.Setenv("BIFROST_DEV", "1")
	t.Setenv("BIFROST_ROUTE_TABLE", "1")

	a := mustNew(t,
		core.Page("/", "./home.tsx"),
		core.Page("/about", "./about.tsx", core.WithClient()),
	)
	defer func() { _ = a.Stop() }()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	a.Handler()

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close stdout pipe: %v", err)
	}
	os.Stdout = oldStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed to read captured stdout: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "Bifrost routes:") {
		t.Errorf("expected route table header in stdout, got:\n%s", out)
	}
	if !strings.Contains(out, "./home.tsx") {
		t.Errorf("expected home route component, got:\n%s", out)
	}
	if !strings.Contains(out, "./about.tsx") {
		t.Errorf("expected about route component, got:\n%s", out)
	}
}

func TestWrapSuppressesRouteTableWhenDisabled(t *testing.T) {
	skipIfNoBun(t)
	t.Setenv("BIFROST_DEV", "1")
	t.Setenv("BIFROST_NO_ROUTE_TABLE", "1")

	a := mustNew(t, core.Page("/", "./home.tsx"))
	defer func() { _ = a.Stop() }()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	a.Handler()

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close stdout pipe: %v", err)
	}
	os.Stdout = oldStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed to read captured stdout: %v", err)
	}

	if strings.Contains(buf.String(), "Bifrost routes:") {
		t.Error("expected route table to be suppressed when BIFROST_NO_ROUTE_TABLE=1")
	}
}

func TestDevModeSetupBeforeStaticDataLoader(t *testing.T) {
	skipIfNoBun(t)
	t.Setenv("BIFROST_DEV", "1")

	loader := func(ctx context.Context) ([]core.StaticPathData, error) {
		return []core.StaticPathData{
			{Path: "/test", Props: map[string]any{"key": "value"}},
		}, nil
	}

	route := core.Page("/blog", "./blog.tsx", core.WithStaticData(loader))

	a := mustNew(t, route)
	defer func() { _ = a.Stop() }()

	config := a.pageConfigs["./blog.tsx"]
	if config == nil {
		t.Fatal("Config not stored")
	}

	if config.Mode != core.ModeStaticPrerender {
		t.Errorf("Expected ModeStaticPrerender, got %v", config.Mode)
	}

	if config.StaticDataLoader == nil {
		t.Error("StaticDataLoader not set")
	}
}
