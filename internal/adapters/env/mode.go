package env

import (
	"os"

	"github.com/3-lines-studio/bifrost/internal/core"
)

const ExportMarkerPath = ".bifrost/.export-mode"

// DetectAppMode returns dev, prod, or export from environment (BIFROST_EXPORT, BIFROST_DEV).
func DetectAppMode() core.Mode {
	if os.Getenv("BIFROST_EXPORT") == "1" {
		return core.ModeExport
	}
	if os.Getenv("BIFROST_DEV") == "1" {
		return core.ModeDev
	}
	return core.ModeProd
}

// IsExportMarkerPresent is true when the build-time export marker file exists.
func IsExportMarkerPresent() bool {
	_, err := os.Stat(ExportMarkerPath)
	return err == nil
}

// DetectMode matches legacy behavior: dev vs prod only (no export env).
// Prefer DetectAppMode for full tri-state behavior.
func DetectMode() core.Mode {
	if os.Getenv("BIFROST_DEV") == "1" {
		return core.ModeDev
	}
	return core.ModeProd
}
