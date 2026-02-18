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
			nil,
			types.PageConfig{
				ComponentPath: "./test.tsx",
				Mode:          types.ModeSSR,
			},
			embed.FS{},
			true,
			nil,
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
			nil,
			types.PageConfig{
				ComponentPath:    "./test.tsx",
				Mode:             types.ModeStaticPrerender,
				StaticDataLoader: loader,
			},
			embed.FS{},
			true,
			nil,
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
		handler := NewHandler(
			nil,
			types.PageConfig{
				ComponentPath: "./test.tsx",
				Mode:          types.ModeClientOnly,
			},
			embed.FS{},
			true,
			nil,
		)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
	})
}

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
			true,
			nil,
		)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rec.Code)
		}
	})
}

func TestHandlerClientOnlyDevMode(t *testing.T) {
	t.Run("ClientOnly in dev mode does not call renderer.Render", func(t *testing.T) {
		renderCalled := false
		mockRenderer := &MockRenderer{}

		trackingRenderer := &TrackingRenderer{
			MockRenderer: mockRenderer,
			renderCalled: &renderCalled,
		}

		handler := NewHandler(
			trackingRenderer,
			types.PageConfig{
				ComponentPath: "./test.tsx",
				Mode:          types.ModeClientOnly,
			},
			embed.FS{},
			true,
			nil,
		)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		defer func() {
			recover()
		}()

		handler.ServeHTTP(rec, req)

		if renderCalled {
			t.Error("ClientOnly in dev mode should not call renderer.Render()")
		}
	})

	t.Run("ClientOnly in dev mode returns HTML shell", func(t *testing.T) {
		mockRenderer := &MockRenderer{}

		handler := NewHandler(
			mockRenderer,
			types.PageConfig{
				ComponentPath: "./test.tsx",
				Mode:          types.ModeClientOnly,
			},
			embed.FS{},
			true,
			nil,
		)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		defer func() {
			recover()
		}()

		handler.ServeHTTP(rec, req)

		body := rec.Body.String()
		if body != "" {
			contentType := rec.Header().Get("Content-Type")
			if contentType != "" && contentType != "text/html; charset=utf-8" {
				t.Errorf("Expected HTML content type, got %s", contentType)
			}
		}
	})
}

type TrackingRenderer struct {
	*MockRenderer
	renderCalled *bool
}

func (t *TrackingRenderer) Render(componentPath string, props map[string]any) (types.RenderedPage, error) {
	*t.renderCalled = true
	return t.MockRenderer.Render(componentPath, props)
}

func (t *TrackingRenderer) Build(entrypoints []string, outdir string) error {
	return t.MockRenderer.Build(entrypoints, outdir)
}
