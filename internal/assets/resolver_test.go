package assets

import (
	"testing"
)

func TestHasSSREntries(t *testing.T) {
	tests := []struct {
		name     string
		manifest *Manifest
		want     bool
	}{
		{
			name:     "nil manifest returns false",
			manifest: nil,
			want:     false,
		},
		{
			name: "empty entries returns false",
			manifest: &Manifest{
				Entries: map[string]ManifestEntry{},
			},
			want: false,
		},
		{
			name: "SSR mode returns true",
			manifest: &Manifest{
				Entries: map[string]ManifestEntry{
					"home": {Mode: "ssr"},
				},
			},
			want: true,
		},
		{
			name: "client-only mode returns false",
			manifest: &Manifest{
				Entries: map[string]ManifestEntry{
					"home":  {Mode: "client-only"},
					"about": {Mode: "client-only"},
				},
			},
			want: false,
		},
		{
			name: "static-prerender mode returns false",
			manifest: &Manifest{
				Entries: map[string]ManifestEntry{
					"blog": {Mode: "static-prerender"},
				},
			},
			want: false,
		},
		{
			name: "mixed modes returns true when SSR present",
			manifest: &Manifest{
				Entries: map[string]ManifestEntry{
					"home":  {Mode: "client-only"},
					"user":  {Mode: "ssr"},
					"about": {Mode: "static-prerender"},
				},
			},
			want: true,
		},
		{
			name: "legacy SSR bundle path returns true",
			manifest: &Manifest{
				Entries: map[string]ManifestEntry{
					"home": {Mode: "", SSR: "/ssr/home.js"},
				},
			},
			want: true,
		},
		{
			name: "legacy empty mode with no SSR returns false",
			manifest: &Manifest{
				Entries: map[string]ManifestEntry{
					"home": {Mode: "", SSR: ""},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasSSREntries(tt.manifest)
			if got != tt.want {
				t.Errorf("HasSSREntries() = %v, want %v", got, tt.want)
			}
		})
	}
}
