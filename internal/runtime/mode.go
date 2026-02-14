package runtime

import (
	"errors"
	"os"
)

type Mode int

const (
	ModeDev Mode = iota
	ModeProd
)

var (
	ErrAssetsFSRequiredInProd    = errors.New("WithAssetsFS is required in production mode (BIFROST_DEV not set)")
	ErrManifestMissingInAssetsFS = errors.New("manifest.json not found in embedded assets")
	ErrEmbeddedRuntimeNotFound   = errors.New("embedded runtime helper not found; run 'bifrost-build' to generate it")
	ErrEmbeddedRuntimeExtraction = errors.New("failed to extract embedded runtime")
	ErrEmbeddedRuntimeStart      = errors.New("failed to start embedded runtime")
)

func GetMode() Mode {
	if os.Getenv("BIFROST_DEV") == "1" {
		return ModeDev
	}
	return ModeProd
}

func IsDev() bool {
	return GetMode() == ModeDev
}

func IsProd() bool {
	return GetMode() == ModeProd
}
