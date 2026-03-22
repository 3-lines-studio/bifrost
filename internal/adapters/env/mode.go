package env

import (
	"os"

	"github.com/3-lines-studio/bifrost/internal/core"
)

const ExportMarkerPath = ".bifrost/.export-mode"

func DetectAppMode() core.Mode {
	if os.Getenv("BIFROST_EXPORT") == "1" {
		return core.ModeExport
	}
	if os.Getenv("BIFROST_DEV") == "1" {
		return core.ModeDev
	}
	return core.ModeProd
}

func IsExportMarkerPresent() bool {
	_, err := os.Stat(ExportMarkerPath)
	return err == nil
}

func DetectMode() core.Mode {
	if os.Getenv("BIFROST_DEV") == "1" {
		return core.ModeDev
	}
	return core.ModeProd
}
