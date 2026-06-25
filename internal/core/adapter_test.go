package core

import "testing"

func TestFrameworkString(t *testing.T) {
	tests := []struct {
		fw   Framework
		want string
	}{
		{FrameworkReact, "react"},
		{Framework(999), "react"},
	}
	for _, tt := range tests {
		if got := tt.fw.String(); got != tt.want {
			t.Errorf("Framework(%d).String() = %q, want %q", tt.fw, got, tt.want)
		}
	}
}

func TestFrameworkFromString(t *testing.T) {
	tests := []struct {
		s    string
		want Framework
	}{
		{"react", FrameworkReact},
		{"", FrameworkReact},
		{"vue", FrameworkReact},
	}
	for _, tt := range tests {
		if got := FrameworkFromString(tt.s); got != tt.want {
			t.Errorf("FrameworkFromString(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestFrameworkFromExtension(t *testing.T) {
	tests := []struct {
		ext  string
		want Framework
	}{
		{".tsx", FrameworkReact},
		{".jsx", FrameworkReact},
		{".ts", FrameworkReact},
		{".js", FrameworkReact},
		{"", FrameworkReact},
	}
	for _, tt := range tests {
		if got := FrameworkFromExtension(tt.ext); got != tt.want {
			t.Errorf("FrameworkFromExtension(%q) = %v, want %v", tt.ext, got, tt.want)
		}
	}
}

func TestFrameworkFromComponentPath(t *testing.T) {
	tests := []struct {
		path string
		want Framework
	}{
		{"./pages/home.tsx", FrameworkReact},
		{"./pages/home.tsx?t=123", FrameworkReact},
		{"/abs/path/page.jsx", FrameworkReact},
		{"unknown", FrameworkReact},
		{"", FrameworkReact},
	}
	for _, tt := range tests {
		if got := FrameworkFromComponentPath(tt.path); got != tt.want {
			t.Errorf("FrameworkFromComponentPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
