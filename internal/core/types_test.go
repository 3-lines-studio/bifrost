package core

import "testing"

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
