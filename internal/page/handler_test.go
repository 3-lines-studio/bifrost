package page

import (
	"context"
	"embed"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/types"
)

func TestHandlerNilRenderer(t *testing.T) {
	t.Run("SSR page with nil renderer returns 500", func(t *testing.T) {
		handler := NewHandler(
			nil, // nil renderer
			types.PageConfig{
				ComponentPath: "./test.tsx",
				Mode:          types.ModeSSR,
			},
			embed.FS{},
			true, // isDev
			nil,  // manifest
		)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("Expected 500, got %d", rec.Code)
		}

		body := rec.Body.String()
		if body == "" {
			t.Error("Expected error body, got empty")
		}
	})

	t.Run("StaticPrerender with StaticDataLoader and nil renderer returns 500", func(t *testing.T) {
		loader := func(ctx context.Context) ([]types.StaticPathData, error) {
			return []types.StaticPathData{
				{Path: "/test", Props: map[string]any{"key": "value"}},
			}, nil
		}

		handler := NewHandler(
			nil, // nil renderer
			types.PageConfig{
				ComponentPath:    "./test.tsx",
				Mode:             types.ModeStaticPrerender,
				StaticDataLoader: loader,
			},
			embed.FS{},
			true, // isDev
			nil,  // manifest
		)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("Expected 500, got %d", rec.Code)
		}

		body := rec.Body.String()
		if body == "" {
			t.Error("Expected error body, got empty")
		}
	})

	t.Run("ClientOnly page with nil renderer still works (no runtime needed)", func(t *testing.T) {
		// Note: ClientOnly pages may fail for other reasons (missing bundles)
		// but they shouldn't fail due to nil renderer specifically
		handler := NewHandler(
			nil, // nil renderer
			types.PageConfig{
				ComponentPath: "./test.tsx",
				Mode:          types.ModeClientOnly,
			},
			embed.FS{},
			true, // isDev
			nil,  // manifest
		)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		// ClientOnly doesn't require renderer, so we just verify it doesn't panic
		// It might return 500 for other reasons (missing bundles) which is fine
	})
}

// MockRenderer is a mock renderer for testing
type MockRenderer struct {
	shouldError bool
}

func (m *MockRenderer) Render(componentPath string, props map[string]any) (types.RenderedPage, error) {
	if m.shouldError {
		return types.RenderedPage{}, errors.New("mock render error")
	}
	return types.RenderedPage{
		Body: "<div>test</div>",
		Head: "",
	}, nil
}

func (m *MockRenderer) Build(entrypoints []string, outdir string) error {
	return nil
}

func TestHandlerWithRenderer(t *testing.T) {
	t.Run("SSR page with renderer succeeds", func(t *testing.T) {
		mockRenderer := &MockRenderer{}

		handler := NewHandler(
			mockRenderer,
			types.PageConfig{
				ComponentPath: "./test.tsx",
				Mode:          types.ModeSSR,
			},
			embed.FS{},
			true, // isDev
			nil,  // manifest
		)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rec.Code)
		}
	})
}
