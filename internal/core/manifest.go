package core

import (
	"encoding/json"
)

type ManifestEntry struct {
	Script       string            `json:"script"`
	CriticalCSS  string            `json:"criticalCSS,omitempty"`
	CSS          string            `json:"css,omitempty"`
	CSSFiles     []string          `json:"cssFiles,omitempty"`
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

type ClientBuildResult struct {
	Script      string   `json:"script"`
	CriticalCSS string   `json:"criticalCSS,omitempty"`
	CSS         string   `json:"css,omitempty"`
	CSSFiles    []string `json:"cssFiles,omitempty"`
	Chunks      []string `json:"chunks,omitempty"`
}

// Assets is an alias for PageArtifacts (legacy name used across the codebase).
type Assets = PageArtifacts

func StylesheetHrefs(css string, cssFiles []string) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(u string) {
		if u == "" {
			return
		}
		if _, ok := seen[u]; ok {
			return
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	add(css)
	for _, u := range cssFiles {
		add(u)
	}
	return out
}

// GetAssets is equivalent to ResolvePageArtifacts. Prefer ResolvePageArtifacts in new code.
func GetAssets(man *Manifest, entryName string) Assets {
	return ResolvePageArtifacts(man, entryName)
}

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
