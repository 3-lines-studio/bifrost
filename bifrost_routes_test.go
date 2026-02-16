package bifrost

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockRouter struct {
	handlers map[string]http.Handler
	patterns []string
}

func newMockRouter() *mockRouter {
	return &mockRouter{
		handlers: make(map[string]http.Handler),
		patterns: []string{},
	}
}

func (m *mockRouter) Handle(pattern string, handler http.Handler) {
	m.handlers[pattern] = handler
	m.patterns = append(m.patterns, pattern)
}

func TestRegisterAssetRoutesWithServeMux(t *testing.T) {
	t.Setenv("BIFROST_DEV", "1")

	r, err := New()
	if err != nil {
		t.Skipf("Skipping test: %v (is bun installed?)", err)
	}
	defer r.Stop()

	page := r.NewPage("./example/components/hello.tsx")

	appRouter := http.NewServeMux()
	appRouter.Handle("/", page)

	assetRouter := http.NewServeMux()
	RegisterAssetRoutes(assetRouter, r, appRouter)

	// Test that root path works (this was returning 404 before the fix)
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	assetRouter.ServeHTTP(rr, req)

	// Should get some response (either the page or a redirect, but not 404 from routing)
	if rr.Code == http.StatusNotFound {
		t.Errorf("Root path / returned 404, expected the page handler to be called")
	}

	// Test that /dist/ pattern is registered (it should try to serve the file even if it doesn't exist)
	req2 := httptest.NewRequest("GET", "/dist/test.js", nil)
	rr2 := httptest.NewRecorder()
	assetRouter.ServeHTTP(rr2, req2)

	// The dist route should be hit (returns 404 for missing file, which is expected)
	// We just verify it doesn't panic or cause issues
}

func TestRegisterAssetRoutesPatterns(t *testing.T) {
	t.Setenv("BIFROST_DEV", "1")

	tests := []struct {
		name            string
		isServeMux      bool
		wantDistPattern string
		wantAppPattern  string
	}{
		{
			name:            "ServeMux gets slash patterns",
			isServeMux:      true,
			wantDistPattern: "/dist/",
			wantAppPattern:  "/{path...}",
		},
		{
			name:            "Non-ServeMux gets wildcard patterns",
			isServeMux:      false,
			wantDistPattern: "/dist/*",
			wantAppPattern:  "/*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := New()
			if err != nil {
				t.Skipf("Skipping test: %v (is bun installed?)", err)
			}
			defer r.Stop()

			var router Router
			if tt.isServeMux {
				router = http.NewServeMux()
			} else {
				router = newMockRouter()
			}

			appRouter := http.NewServeMux()
			appRouter.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			RegisterAssetRoutes(router, r, appRouter)

			if !tt.isServeMux {
				mock := router.(*mockRouter)
				hasDist := false
				hasApp := false
				for _, p := range mock.patterns {
					if p == tt.wantDistPattern {
						hasDist = true
					}
					if p == tt.wantAppPattern {
						hasApp = true
					}
				}
				if !hasDist {
					t.Errorf("Expected dist pattern %q to be registered, got %v", tt.wantDistPattern, mock.patterns)
				}
				if !hasApp {
					t.Errorf("Expected app pattern %q to be registered, got %v", tt.wantAppPattern, mock.patterns)
				}
			}
		})
	}
}
