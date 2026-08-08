package core

import (
	"context"
	"net/http"
	"testing"
)

func TestPageModeIsStatic(t *testing.T) {
	tests := []struct {
		mode PageMode
		want bool
	}{
		{mode: ModeSSR, want: false},
		{mode: ModeClientOnly, want: true},
		{mode: ModeStaticPrerender, want: true},
	}

	for _, tt := range tests {
		if got := tt.mode.IsStatic(); got != tt.want {
			t.Errorf("mode %v IsStatic() = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

func TestPageModeNeedsSSRBundle(t *testing.T) {
	tests := []struct {
		mode PageMode
		want bool
	}{
		{mode: ModeSSR, want: true},
		{mode: ModeClientOnly, want: false},
		{mode: ModeStaticPrerender, want: false},
	}

	for _, tt := range tests {
		if got := tt.mode.NeedsSSRBundle(); got != tt.want {
			t.Errorf("mode %v NeedsSSRBundle() = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

func TestPageModeBuildLabel(t *testing.T) {
	tests := []struct {
		mode PageMode
		want string
	}{
		{mode: ModeSSR, want: "ssr"},
		{mode: ModeClientOnly, want: "client"},
		{mode: ModeStaticPrerender, want: "static"},
	}

	for _, tt := range tests {
		if got := tt.mode.BuildLabel(); got != tt.want {
			t.Errorf("mode %v BuildLabel() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestPageModeRenderAction(t *testing.T) {
	tests := []struct {
		mode PageMode
		want PageAction
	}{
		{mode: ModeSSR, want: ActionRenderSSR},
		{mode: ModeClientOnly, want: ActionRenderClientOnlyShell},
		{mode: ModeStaticPrerender, want: ActionRenderStaticPrerender},
	}

	for _, tt := range tests {
		if got := tt.mode.RenderAction(); got != tt.want {
			t.Errorf("mode %v RenderAction() = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

func TestPageModeDevAction(t *testing.T) {
	tests := []struct {
		name        string
		mode        PageMode
		hasRenderer bool
		want        PageDecision
	}{
		{
			name:        "ssr needs setup",
			mode:        ModeSSR,
			hasRenderer: false,
			want:        PageDecision{Action: ActionNeedsSetup, NeedsSetup: true},
		},
		{
			name:        "client-only renders without renderer",
			mode:        ModeClientOnly,
			hasRenderer: false,
			want:        PageDecision{Action: ActionRenderClientOnlyShell},
		},
		{
			name:        "static prerender renders without renderer",
			mode:        ModeStaticPrerender,
			hasRenderer: false,
			want:        PageDecision{Action: ActionRenderStaticPrerender},
		},
		{
			name:        "client-only with renderer still needs setup",
			mode:        ModeClientOnly,
			hasRenderer: true,
			want:        PageDecision{Action: ActionNeedsSetup, NeedsSetup: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mode.DevAction(tt.hasRenderer); got != tt.want {
				t.Errorf("mode %v DevAction(%v) = %+v, want %+v", tt.mode, tt.hasRenderer, got, tt.want)
			}
		})
	}
}

func TestPageConfigOptions(t *testing.T) {
	t.Run("WithLoader sets PropsLoader", func(t *testing.T) {
		loader := func(*http.Request) (any, error) { return nil, nil }
		route := Page("/", "./pages/home.tsx", WithLoader(loader))
		config := mustPageConfig(t, route)
		if config.PropsLoader == nil {
			t.Fatal("expected PropsLoader to be set")
		}
	})

	t.Run("WithPreLoader sets PreLoader", func(t *testing.T) {
		loader := func(*http.Request) (PreLoaderResult, error) { return PreLoaderResult{}, nil }
		route := Page("/", "./pages/home.tsx", WithPreLoader(loader))
		config := mustPageConfig(t, route)
		if config.PreLoader == nil {
			t.Fatal("expected PreLoader to be set")
		}
	})

	t.Run("WithClient sets client-only mode", func(t *testing.T) {
		route := Page("/", "./pages/home.tsx", WithClient())
		config := mustPageConfig(t, route)
		if config.Mode != ModeClientOnly {
			t.Fatalf("expected ModeClientOnly, got %v", config.Mode)
		}
	})

	t.Run("WithStatic sets static prerender mode", func(t *testing.T) {
		route := Page("/", "./pages/home.tsx", WithStatic())
		config := mustPageConfig(t, route)
		if config.Mode != ModeStaticPrerender {
			t.Fatalf("expected ModeStaticPrerender, got %v", config.Mode)
		}
	})

	t.Run("WithStaticData sets mode and loader", func(t *testing.T) {
		loader := func(context.Context) ([]StaticPathData, error) { return nil, nil }
		route := Page("/", "./pages/home.tsx", WithStaticData(loader))
		config := mustPageConfig(t, route)
		if config.Mode != ModeStaticPrerender {
			t.Fatalf("expected ModeStaticPrerender, got %v", config.Mode)
		}
		if config.StaticDataLoader == nil {
			t.Fatal("expected StaticDataLoader to be set")
		}
	})

	t.Run("WithHTMLLang sets HTMLLang", func(t *testing.T) {
		route := Page("/", "./pages/home.tsx", WithHTMLLang("fr"))
		config := mustPageConfig(t, route)
		if config.HTMLLang != "fr" {
			t.Fatalf("expected HTMLLang fr, got %q", config.HTMLLang)
		}
	})

	t.Run("WithHTMLClass sets HTMLClass", func(t *testing.T) {
		route := Page("/", "./pages/home.tsx", WithHTMLClass("dark"))
		config := mustPageConfig(t, route)
		if config.HTMLClass != "dark" {
			t.Fatalf("expected HTMLClass dark, got %q", config.HTMLClass)
		}
	})
}
