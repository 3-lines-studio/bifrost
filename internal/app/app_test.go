package app

import (
	"context"
	"embed"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

var testFS embed.FS

func bunAvailable() bool {
	_, err := exec.LookPath("bun")
	return err == nil
}

func skipIfNoBun(t *testing.T) {
	if !bunAvailable() {
		t.Skip("bun not available, skipping integration test")
	}
}

func TestNewCreatesApp(t *testing.T) {
	skipIfNoBun(t)
	t.Setenv("BIFROST_DEV", "1")

	a := New(testFS)
	defer func() { _ = a.Stop() }()

	if a == nil {
		t.Error("New() returned nil app")
	}
}

func TestStrictProductionRequirements(t *testing.T) {
	t.Setenv("BIFROST_DEV", "")

	t.Run("production without assets FS panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic in production without assets FS, got nil")
			}
		}()
		New(testFS)
	})
}

func TestAppWrapWithServeMux(t *testing.T) {
	skipIfNoBun(t)
	t.Setenv("BIFROST_DEV", "1")

	a := New(testFS, core.Page("/", "./example/components/hello.tsx"))
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

	a := New(testFS, core.Page("/", "./test.tsx"))
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
			a := New(testFS, core.Page("/", "./test.tsx"))
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

	a := New(testFS, core.Page("/", "./test.tsx"))
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

		a := New(testFS, core.Page("/test", "./test.tsx", core.WithLoader(func(*http.Request) (map[string]any, error) {
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

		a := New(testFS, core.Page("/test", "./test.tsx", core.WithClient()))
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

		a := New(testFS, core.Page("/test", "./test.tsx", core.WithStatic()))
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

	a := New(testFS, route)
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

	a := New(testFS, route)
	defer func() { _ = a.Stop() }()

	config := a.pageConfigs["./blog.tsx"]
	if config == nil {
		t.Fatal("Config not stored")
	}

	if config.StaticDataLoader == nil {
		t.Error("StaticDataLoader not set")
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

	a := New(testFS, route)
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
