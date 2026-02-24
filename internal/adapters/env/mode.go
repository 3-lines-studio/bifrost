package env

import (
	"os"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func DetectMode() core.Mode {
	if os.Getenv("BIFROST_DEV") == "1" {
		return core.ModeDev
	}
	return core.ModeProd
}
