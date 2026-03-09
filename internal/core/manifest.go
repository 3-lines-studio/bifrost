package core

import (
	"encoding/json"
)

type ManifestEntry struct {
	Script       string            `json:"script"`
	CSS          string            `json:"css,omitempty"`
	Chunks       []string          `json:"chunks,omitempty"`
	Static       bool              `json:"static,omitempty"`
	SSR          string            `json:"ssr,omitempty"`
	Mode         string            `json:"mode,omitempty"`
	HTML         string            `json:"html,omitempty"`
	StaticRoutes map[string]string `json:"staticRoutes,omitempty"`
}

type Manifest struct {
	Entries map[string]ManifestEntry `json:"entries"`
	Chunks  map[string]string        `json:"chunks,omitempty"`
}

func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

type Assets struct {
	Script   string
	CSS      string
	Chunks   []string
	IsStatic bool
	SSRPath  string
}

func GetAssets(man *Manifest, entryName string) Assets {
	if man != nil {
		if entry, ok := man.Entries[entryName]; ok && entry.Script != "" {
			return Assets{
				Script:   entry.Script,
				CSS:      entry.CSS,
				Chunks:   entry.Chunks,
				IsStatic: entry.Static,
				SSRPath:  entry.SSR,
			}
		}
	}
	return Assets{
		Script: "/dist/" + entryName + ".js",
		CSS:    "/dist/" + entryName + ".css",
	}
}

// HasSSREntries returns true if any manifest entry requires the SSR runtime
// at production time (mode="ssr"). Static pages with SSR bundles (mode="static")
// do NOT need runtime in production as they are pre-rendered at build time.
func HasSSREntries(man *Manifest) bool {
	if man == nil {
		return false
	}
	for _, entry := range man.Entries {
		if entry.Mode == "ssr" {
			return true
		}
	}
	return false
}

// HasSSRBundles returns true if any manifest entry has an SSR bundle.
// This is used for build-time operations (export mode) where static pages
// need SSR bundles for prerendering, but don't need runtime in production.
func HasSSRBundles(man *Manifest) bool {
	if man == nil {
		return false
	}
	for _, entry := range man.Entries {
		if entry.SSR != "" {
			return true
		}
	}
	return false
}
