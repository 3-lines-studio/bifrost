package bifrost

import (
	"errors"
	"net/http"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/runtime"
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

			mode := runtime.GetMode()
			isDev := mode == runtime.ModeDev
			isProd := mode == runtime.ModeProd

			if isDev != tt.wantDev {
				t.Errorf("IsDev() = %v, want %v", isDev, tt.wantDev)
			}
			if isProd != tt.wantProd {
				t.Errorf("IsProd() = %v, want %v", isProd, tt.wantProd)
			}
		})
	}
}

func TestStrictProductionRequirements(t *testing.T) {
	// Set production mode
	t.Setenv("BIFROST_DEV", "")

	// Test 1: Production without WithAssetsFS should fail
	t.Run("production without assets FS fails", func(t *testing.T) {
		_, err := New()
		if err == nil {
			t.Error("Expected error in production without WithAssetsFS, got nil")
		}
		if !errors.Is(err, runtime.ErrAssetsFSRequiredInProd) {
			t.Errorf("Expected ErrAssetsFSRequiredInProd, got: %v", err)
		}
	})
}

func TestPageOptions(t *testing.T) {
	t.Run("WithPropsLoader creates handler", func(t *testing.T) {
		// This test requires a renderer, so we skip if bun is not available
		t.Setenv("BIFROST_DEV", "1")

		r, err := New()
		if err != nil {
			t.Skipf("Skipping test: %v (is bun installed?)", err)
		}
		defer r.Stop()

		loader := func(*http.Request) (map[string]any, error) {
			return map[string]any{"test": "value"}, nil
		}

		handler := r.NewPage("./test.tsx", WithPropsLoader(loader))
		if handler == nil {
			t.Error("NewPage returned nil handler")
		}
	})

	t.Run("WithClientOnly creates handler", func(t *testing.T) {
		t.Setenv("BIFROST_DEV", "1")

		r, err := New()
		if err != nil {
			t.Skipf("Skipping test: %v (is bun installed?)", err)
		}
		defer r.Stop()

		handler := r.NewPage("./test.tsx", WithClientOnly())
		if handler == nil {
			t.Error("NewPage returned nil handler")
		}
	})

	t.Run("NewPage returns http.Handler", func(t *testing.T) {
		t.Setenv("BIFROST_DEV", "1")

		r, err := New()
		if err != nil {
			t.Skipf("Skipping test: %v (is bun installed?)", err)
		}
		defer r.Stop()

		handler := r.NewPage("./test.tsx")

		// Verify it implements http.Handler
		var _ http.Handler = handler
	})

	t.Run("WithStaticPrerender creates handler", func(t *testing.T) {
		t.Setenv("BIFROST_DEV", "1")

		r, err := New()
		if err != nil {
			t.Skipf("Skipping test: %v (is bun installed?)", err)
		}
		defer r.Stop()

		handler := r.NewPage("./test.tsx", WithStaticPrerender())
		if handler == nil {
			t.Error("NewPage returned nil handler")
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
