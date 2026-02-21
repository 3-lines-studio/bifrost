package bifrost

import (
	"net/http"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestModeDetection(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		wantDev  bool
		wantProd bool
	}{
		{
			name:     "dev mode with 1",
			envValue: "1",
			wantDev:  true,
			wantProd: false,
		},
		{
			name:     "prod mode with empty",
			envValue: "",
			wantDev:  false,
			wantProd: true,
		},
		{
			name:     "prod mode with 0",
			envValue: "0",
			wantDev:  false,
			wantProd: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BIFROST_DEV", tt.envValue)

			mode := detectMode()
			isDev := mode == core.ModeDev
			isProd := mode == core.ModeProd

			if isDev != tt.wantDev {
				t.Errorf("IsDev() = %v, want %v", isDev, tt.wantDev)
			}
			if isProd != tt.wantProd {
				t.Errorf("mode == ModeProd = %v, want %v", isProd, tt.wantProd)
			}
		})
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

func TestPageOptions(t *testing.T) {
	t.Run("WithLoader creates route with loader", func(t *testing.T) {
		route := Page("/test", "./test.tsx", WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{"test": "value"}, nil
		}))

		if route.Pattern != "/test" {
			t.Errorf("Expected pattern '/test', got '%s'", route.Pattern)
		}

		if len(route.Options) != 1 {
			t.Errorf("Expected 1 option, got %d", len(route.Options))
		}
	})

	t.Run("WithClient creates route with client option", func(t *testing.T) {
		route := Page("/about", "./about.tsx", WithClient())

		if route.Pattern != "/about" {
			t.Errorf("Expected pattern '/about', got '%s'", route.Pattern)
		}

		if len(route.Options) != 1 {
			t.Errorf("Expected 1 option, got %d", len(route.Options))
		}
	})

	t.Run("Page creates route without options", func(t *testing.T) {
		route := Page("/", "./home.tsx")

		if route.Pattern != "/" {
			t.Errorf("Expected pattern '/', got '%s'", route.Pattern)
		}

		if len(route.Options) != 0 {
			t.Errorf("Expected 0 options, got %d", len(route.Options))
		}
	})

	t.Run("WithStatic creates route with static option", func(t *testing.T) {
		route := Page("/blog", "./blog.tsx", WithStatic())

		if route.Pattern != "/blog" {
			t.Errorf("Expected pattern '/blog', got '%s'", route.Pattern)
		}

		if len(route.Options) != 1 {
			t.Errorf("Expected 1 option, got %d", len(route.Options))
		}
	})
}

func TestPageModeTypes(t *testing.T) {
	t.Run("SSR page has correct mode", func(t *testing.T) {
		skipIfNoBun(t)
		t.Setenv("BIFROST_DEV", "1")

		app := New(testFS, Page("/test", "./test.tsx", WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{}, nil
		})))
		defer func() { _ = app.Stop() }()

		config := app.pageConfigs["./test.tsx"]
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

		app := New(testFS, Page("/test", "./test.tsx", WithClient()))
		defer func() { _ = app.Stop() }()

		config := app.pageConfigs["./test.tsx"]
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

		app := New(testFS, Page("/test", "./test.tsx", WithStatic()))
		defer func() { _ = app.Stop() }()

		config := app.pageConfigs["./test.tsx"]
		if config == nil {
			t.Fatal("Config not stored")
		}
		if config.Mode != core.ModeStaticPrerender {
			t.Errorf("Expected ModeStaticPrerender, got %v", config.Mode)
		}
	})
}

func TestRedirectErrorInterface(t *testing.T) {
	tests := []struct {
		name         string
		redirectErr  *mockRedirectError
		expectedURL  string
		expectedCode int
	}{
		{
			name:         "custom 301 redirect",
			redirectErr:  &mockRedirectError{url: "/new-path", status: http.StatusMovedPermanently},
			expectedURL:  "/new-path",
			expectedCode: http.StatusMovedPermanently,
		},
		{
			name:         "custom 302 redirect",
			redirectErr:  &mockRedirectError{url: "/temp-path", status: http.StatusFound},
			expectedURL:  "/temp-path",
			expectedCode: http.StatusFound,
		},
		{
			name:         "zero status defaults to 302",
			redirectErr:  &mockRedirectError{url: "/default-path", status: 0},
			expectedURL:  "/default-path",
			expectedCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.redirectErr.RedirectURL() != tt.expectedURL {
				t.Errorf("RedirectURL() = %q, want %q", tt.redirectErr.RedirectURL(), tt.expectedURL)
			}
			if tt.redirectErr.RedirectStatusCode() != tt.expectedCode {
				t.Errorf("RedirectStatusCode() = %d, want %d", tt.redirectErr.RedirectStatusCode(), tt.expectedCode)
			}
		})
	}
}
