package bifrost

import "runtime/debug"

const modulePath = "github.com/3-lines-studio/bifrost"

// Version identifies the Bifrost build tool in generated manifests. Release
// binaries derive it from Go module build information. It remains mutable so a
// release pipeline can set it with -ldflags when needed.
var Version = detectedVersion()

func detectedVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "devel"
	}
	if info.Main.Path == modulePath && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	for _, dependency := range info.Deps {
		if dependency.Path == modulePath && dependency.Version != "" {
			return dependency.Version
		}
	}
	var revision string
	modified := false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" {
		return "devel"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if modified {
		revision += "+dirty"
	}
	return "devel+" + revision
}
