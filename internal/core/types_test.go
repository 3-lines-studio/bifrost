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
		WithLoader(func(*http.Request) (any, error) {
			return map[string]any{"locale": "en"}, nil
		}),
		WithDeferredLoader(func(*http.Request) (any, error) {
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
		m, ok := result.(map[string]any)
		if !ok {
			t.Fatal("expected map result")
		}
		if len(m) != 4 {
			t.Fatalf("expected 4 keys, got %d", len(m))
		}
		if m["locale"] != "en" {
			t.Errorf("expected locale=en, got %v", m["locale"])
		}
		if m["user"] != "alice" {
			t.Errorf("expected user=alice, got %v", m["user"])
		}
	})

	t.Run("deferred overrides sync on key collision", func(t *testing.T) {
		result := MergeProps(
			map[string]any{"key": "sync"},
			map[string]any{"key": "deferred"},
		)
		m, ok := result.(map[string]any)
		if !ok {
			t.Fatal("expected map result")
		}
		if m["key"] != "deferred" {
			t.Errorf("expected deferred to override, got %v", m["key"])
		}
	})

	t.Run("empty sync", func(t *testing.T) {
		result := MergeProps(nil, map[string]any{"user": "alice"})
		m, ok := result.(map[string]any)
		if !ok {
			t.Fatal("expected map result")
		}
		if m["user"] != "alice" {
			t.Errorf("expected user=alice, got %v", m["user"])
		}
	})

	t.Run("empty deferred", func(t *testing.T) {
		result := MergeProps(map[string]any{"locale": "en"}, nil)
		m, ok := result.(map[string]any)
		if !ok {
			t.Fatal("expected map result")
		}
		if m["locale"] != "en" {
			t.Errorf("expected locale=en, got %v", m["locale"])
		}
	})

	t.Run("both empty", func(t *testing.T) {
		result := MergeProps(nil, nil)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("struct sync empty deferred", func(t *testing.T) {
		type myProps struct {
			Name string `json:"name"`
		}
		original := myProps{Name: "World"}
		result := MergeProps(original, nil)
		// Struct returned as-is when deferred is nil
		p, ok := result.(myProps)
		if !ok {
			t.Fatalf("expected myProps struct, got %T", result)
		}
		if p.Name != "World" {
			t.Errorf("expected name=World, got %v", p.Name)
		}
	})

	t.Run("struct sync struct deferred", func(t *testing.T) {
		type p1 struct {
			Name string `json:"name"`
		}
		type p2 struct {
			Count int `json:"count"`
		}
		result := MergeProps(p1{Name: "World"}, p2{Count: 42})
		m, ok := result.(map[string]any)
		if !ok {
			t.Fatal("expected map result from structs")
		}
		if m["name"] != "World" {
			t.Errorf("expected name=World, got %v", m["name"])
		}
		if m["count"] != float64(42) {
			t.Errorf("expected count=42, got %v", m["count"])
		}
	})

	t.Run("struct with map deferred", func(t *testing.T) {
		type p1 struct {
			Name string `json:"name"`
		}
		result := MergeProps(p1{Name: "sync"}, map[string]any{"name": "deferred"})
		m, ok := result.(map[string]any)
		if !ok {
			t.Fatal("expected map result")
		}
		if m["name"] != "deferred" {
			t.Errorf("expected deferred to override, got %v", m["name"])
		}
	})

	t.Run("map sync struct deferred adds new key", func(t *testing.T) {
		type deferred struct {
			Count int `json:"count"`
		}
		result := MergeProps(map[string]any{"name": "sync"}, deferred{Count: 7})
		m, ok := result.(map[string]any)
		if !ok {
			t.Fatal("expected map result")
		}
		if m["name"] != "sync" {
			t.Errorf("expected name=sync, got %v", m["name"])
		}
		if m["count"] != float64(7) {
			t.Errorf("expected count=7, got %v", m["count"])
		}
	})

	t.Run("struct struct collision deferred wins", func(t *testing.T) {
		type syncProps struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}
		type deferredProps struct {
			Name string `json:"name"`
		}
		result := MergeProps(syncProps{Name: "sync", Count: 1}, deferredProps{Name: "deferred"})
		m, ok := result.(map[string]any)
		if !ok {
			t.Fatal("expected map result")
		}
		if m["name"] != "deferred" {
			t.Errorf("expected deferred to override name, got %v", m["name"])
		}
		if m["count"] != float64(1) {
			t.Errorf("expected count=1, got %v", m["count"])
		}
	})

	t.Run("non map struct fallback returns other", func(t *testing.T) {
		result := MergeProps([]int{1, 2}, map[string]any{"name": "ok"})
		m, ok := result.(map[string]any)
		if !ok {
			t.Fatal("expected map result")
		}
		if m["name"] != "ok" {
			t.Errorf("expected name=ok, got %v", m["name"])
		}
	})
}
