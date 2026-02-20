package core

type ManifestEntryClassification struct {
	ModeStr  string
	IsStatic bool
	HTMLPath string
}

func ClassifyPageMode(mode PageMode, hasRoutes bool) ManifestEntryClassification {
	result := ManifestEntryClassification{
		ModeStr:  "ssr",
		IsStatic: false,
		HTMLPath: "",
	}

	switch mode {
	case ModeClientOnly:
		result.ModeStr = "client-only"
		result.IsStatic = true
		result.HTMLPath = "" // Will be set by caller with entry name
	case ModeStaticPrerender:
		result.ModeStr = "static-prerender"
		result.IsStatic = true
		if !hasRoutes {
			result.HTMLPath = "" // Will be set by caller with entry name
		}
	}

	return result
}

func BuildHTMLPath(entryName string) string {
	return "/pages/" + entryName + "/index.html"
}

type BuildDecisionInput struct {
	Mode      PageMode
	HasRoutes bool
	EntryName string
}

type BuildDecision struct {
	ModeStr  string
	IsStatic bool
	HTMLPath string
}

func DecideBuildEntry(input BuildDecisionInput) BuildDecision {
	classification := ClassifyPageMode(input.Mode, input.HasRoutes)

	htmlPath := classification.HTMLPath
	if htmlPath == "" && classification.IsStatic {
		htmlPath = BuildHTMLPath(input.EntryName)
	}

	return BuildDecision{
		ModeStr:  classification.ModeStr,
		IsStatic: classification.IsStatic,
		HTMLPath: htmlPath,
	}
}

func ShouldBuildSSR(mode PageMode) bool {
	return mode != ModeClientOnly && mode != ModeStaticPrerender
}

func IsStaticMode(mode PageMode) bool {
	return mode == ModeClientOnly || mode == ModeStaticPrerender
}
