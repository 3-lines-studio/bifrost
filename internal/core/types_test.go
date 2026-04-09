package core

import (
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

func TestWithDeferredLoader(t *testing.T) {
	route := Page("/test", "./test.tsx",
		WithLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{"locale": "en"}, nil
		}),
		WithDeferredLoader(func(*http.Request) (map[string]any, error) {
			return map[string]any{"user": "test"}, nil
		}),
	)

	config := PageConfigFromRoute(route)
	if config.PropsLoader == nil {
		t.Fatal("expected PropsLoader to be set")
	}
	if config.DeferredPropsLoader == nil {
		t.Fatal("expected DeferredPropsLoader to be set")
	}
}

func TestMergeProps(t *testing.T) {
	t.Run("both non-nil", func(t *testing.T) {
		result := MergeProps(
			map[string]any{"locale": "en", "href": "/"},
			map[string]any{"user": "alice", "carts": 3},
		)
		if len(result) != 4 {
			t.Fatalf("expected 4 keys, got %d", len(result))
		}
		if result["locale"] != "en" {
			t.Errorf("expected locale=en, got %v", result["locale"])
		}
		if result["user"] != "alice" {
			t.Errorf("expected user=alice, got %v", result["user"])
		}
	})

	t.Run("deferred overrides sync on key collision", func(t *testing.T) {
		result := MergeProps(
			map[string]any{"key": "sync"},
			map[string]any{"key": "deferred"},
		)
		if result["key"] != "deferred" {
			t.Errorf("expected deferred to override, got %v", result["key"])
		}
	})

	t.Run("empty sync", func(t *testing.T) {
		result := MergeProps(nil, map[string]any{"user": "alice"})
		if result["user"] != "alice" {
			t.Errorf("expected user=alice, got %v", result["user"])
		}
	})

	t.Run("empty deferred", func(t *testing.T) {
		result := MergeProps(map[string]any{"locale": "en"}, nil)
		if result["locale"] != "en" {
			t.Errorf("expected locale=en, got %v", result["locale"])
		}
	})

	t.Run("both empty", func(t *testing.T) {
		result := MergeProps(nil, nil)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})
}
