package core

type BuildDecision struct {
	ModeStr  string
	IsStatic bool
	HTMLPath string
}

func DecideBuildEntry(mode PageMode, hasRoutes bool, entryName string) BuildDecision {
	result := BuildDecision{
		ModeStr:  "ssr",
		IsStatic: false,
		HTMLPath: "",
	}

	switch mode {
	case ModeClientOnly:
		result.ModeStr = "client-only"
		result.IsStatic = true
		result.HTMLPath = "/pages/" + entryName + "/index.html"
	case ModeStaticPrerender:
		result.ModeStr = "static-prerender"
		result.IsStatic = true
		if !hasRoutes {
			result.HTMLPath = "/pages/" + entryName + "/index.html"
		}
	}

	return result
}

func ShouldBuildSSR(mode PageMode) bool {
	return mode != ModeClientOnly && mode != ModeStaticPrerender
}

func IsStaticMode(mode PageMode) bool {
	return mode == ModeClientOnly || mode == ModeStaticPrerender
}
