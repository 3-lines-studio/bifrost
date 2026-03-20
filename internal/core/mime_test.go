package core

import "testing"

func TestGetContentType(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"foo.webp", "image/webp"},
		{"foo.WEBP", "image/webp"},
		{"path/to/asset.webp", "image/webp"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := GetContentType(tt.path); got != tt.want {
				t.Errorf("GetContentType(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
