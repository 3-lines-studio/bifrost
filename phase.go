package bifrost

import "os"

// Building reports whether the app is running under the Bifrost describe or
// static-generation protocol. Main should return immediately after New in this
// mode, before opening databases, listeners, or other services.
func Building() bool { return os.Getenv(buildPhaseEnv) != "" }
