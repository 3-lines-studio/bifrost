package bifrost

import "os"

func Building() bool { return os.Getenv(buildPhaseEnv) != "" }
