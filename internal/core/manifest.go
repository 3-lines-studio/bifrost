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

func GetAssets(man *Manifest, entryName string) (scriptSrc, cssHref string, chunks []string, isStatic bool, ssrPath string) {
	if man != nil && man.Entries[entryName].Script != "" {
		entry := man.Entries[entryName]
		return entry.Script, entry.CSS, entry.Chunks, entry.Static, entry.SSR
	}
	return "/dist/" + entryName + ".js", "/dist/" + entryName + ".css", nil, false, ""
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
