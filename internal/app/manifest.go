package app

import (
	"fmt"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func validateProductionManifest(configs []core.PageConfig, manifest *core.Manifest) error {
	if manifest == nil {
		return fmt.Errorf("manifest is nil")
	}

	seen := make(map[string]struct{}, len(configs))
	for _, config := range configs {
		entryName := core.EntryNameForPath(config.ComponentPath)
		if _, ok := seen[entryName]; ok {
			continue
		}
		seen[entryName] = struct{}{}

		entry, ok := manifest.Entries[entryName]
		if !ok {
			return fmt.Errorf("missing manifest entry %q for component %q; run 'bifrost build'", entryName, config.ComponentPath)
		}
		if entry.Mode != config.Mode.BuildLabel() {
			return fmt.Errorf(
				"manifest entry %q has mode %q, want %q for component %q; run 'bifrost build'",
				entryName,
				entry.Mode,
				config.Mode.BuildLabel(),
				config.ComponentPath,
			)
		}
		if entry.Script == "" {
			return fmt.Errorf("manifest entry %q has no client script; run 'bifrost build'", entryName)
		}
		switch config.Mode {
		case core.ModeSSR:
			if entry.SSR == "" {
				return fmt.Errorf("manifest entry %q has no SSR bundle; run 'bifrost build'", entryName)
			}
		case core.ModeClientOnly:
			if entry.HTML == "" {
				return fmt.Errorf("manifest entry %q has no client HTML shell; run 'bifrost build'", entryName)
			}
		}
	}
	return nil
}
