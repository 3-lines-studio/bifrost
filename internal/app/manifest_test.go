package app

import (
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestValidateProductionManifest(t *testing.T) {
	ssrConfig := core.PageConfig{ComponentPath: "./pages/home.tsx", Mode: core.ModeSSR}
	entryName := core.EntryNameForPath(ssrConfig.ComponentPath)
	validSSR := core.ManifestEntry{
		Mode:   "ssr",
		Script: "/dist/home.js",
		SSR:    "/ssr/home.js",
	}

	tests := []struct {
		name     string
		configs  []core.PageConfig
		manifest *core.Manifest
		wantErr  string
	}{
		{name: "valid SSR", configs: []core.PageConfig{ssrConfig}, manifest: &core.Manifest{Entries: map[string]core.ManifestEntry{entryName: validSSR}}},
		{name: "nil manifest", configs: []core.PageConfig{ssrConfig}, wantErr: "manifest is nil"},
		{name: "missing entry", configs: []core.PageConfig{ssrConfig}, manifest: &core.Manifest{Entries: map[string]core.ManifestEntry{}}, wantErr: "missing manifest entry"},
		{name: "stale mode", configs: []core.PageConfig{ssrConfig}, manifest: &core.Manifest{Entries: map[string]core.ManifestEntry{entryName: {Mode: "client", Script: "/dist/home.js"}}}, wantErr: "has mode"},
		{name: "missing script", configs: []core.PageConfig{ssrConfig}, manifest: &core.Manifest{Entries: map[string]core.ManifestEntry{entryName: {Mode: "ssr", SSR: "/ssr/home.js"}}}, wantErr: "no client script"},
		{name: "missing SSR bundle", configs: []core.PageConfig{ssrConfig}, manifest: &core.Manifest{Entries: map[string]core.ManifestEntry{entryName: {Mode: "ssr", Script: "/dist/home.js"}}}, wantErr: "no SSR bundle"},
		{
			name:    "missing client shell",
			configs: []core.PageConfig{{ComponentPath: "./pages/home.tsx", Mode: core.ModeClientOnly}},
			manifest: &core.Manifest{Entries: map[string]core.ManifestEntry{
				entryName: {Mode: "client", Script: "/dist/home.js"},
			}},
			wantErr: "no client HTML shell",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProductionManifest(tt.configs, tt.manifest)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateProductionManifest() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateProductionManifest() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
