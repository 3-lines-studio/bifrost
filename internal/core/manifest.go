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

type Assets struct {
	Script      string
	CriticalCSS string
	CSS         string
	CSSFiles    []string
	Chunks      []string
	IsStatic    bool
	SSRPath     string
}

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

func GetAssets(man *Manifest, entryName string) Assets {
	if man != nil {
		if entry, ok := man.Entries[entryName]; ok && entry.Script != "" {
			return Assets{
				Script:      entry.Script,
				CriticalCSS: entry.CriticalCSS,
				CSS:         entry.CSS,
				CSSFiles:    entry.CSSFiles,
				Chunks:      entry.Chunks,
				IsStatic:    entry.Static,
				SSRPath:     entry.SSR,
			}
		}
	}
	return Assets{
		Script: "/dist/" + entryName + ".js",
		CSS:    "/dist/" + entryName + ".css",
	}
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
