package core

type PageAction int

const (
	ActionServeStaticFile PageAction = iota
	ActionServeRouteFile
	ActionNotFound
	ActionNeedsSetup
	ActionRenderClientOnlyShell
	ActionRenderStaticPrerender
	ActionRenderSSR
)

type PageRequest struct {
	IsDev       bool
	Mode        PageMode
	RequestPath string
	HasManifest bool
	EntryName   string
	StaticPath  string
	HasRenderer bool
}

type PageDecision struct {
	Action     PageAction
	HTMLPath   string
	StaticPath string
	NeedsSetup bool
}

func DecidePageAction(req PageRequest, entry *ManifestEntry) PageDecision {
	// Production mode: serve static files when available
	if !req.IsDev {
		return decideProductionAction(req, entry)
	}

	// Development mode: check if setup is needed
	needsSetup := req.Mode != ModeClientOnly && req.Mode != ModeStaticPrerender
	if (req.Mode == ModeClientOnly || req.Mode == ModeStaticPrerender) && req.HasRenderer {
		needsSetup = true
	}

	if needsSetup {
		return PageDecision{Action: ActionNeedsSetup, NeedsSetup: true}
	}

	// Dev mode: render based on page type
	switch req.Mode {
	case ModeClientOnly:
		return PageDecision{Action: ActionRenderClientOnlyShell}
	case ModeStaticPrerender:
		return PageDecision{Action: ActionRenderStaticPrerender}
	default:
		return PageDecision{Action: ActionRenderSSR}
	}
}

func decideProductionAction(req PageRequest, entry *ManifestEntry) PageDecision {
	switch req.Mode {
	case ModeClientOnly:
		return decideClientOnlyAction(req, entry)
	case ModeStaticPrerender:
		return decideStaticPrerenderAction(req, entry, NormalizePath(req.RequestPath))
	default:
		return PageDecision{Action: ActionRenderSSR}
	}
}

func decideClientOnlyAction(req PageRequest, entry *ManifestEntry) PageDecision {
	if entry != nil && entry.HTML != "" {
		return PageDecision{Action: ActionServeStaticFile, StaticPath: entry.HTML}
	}
	if req.StaticPath != "" {
		return PageDecision{Action: ActionServeStaticFile, StaticPath: req.StaticPath}
	}
	return PageDecision{Action: ActionNotFound}
}

func decideStaticPrerenderAction(req PageRequest, entry *ManifestEntry, normalizedPath string) PageDecision {
	if req.HasManifest && entry != nil && entry.StaticRoutes != nil {
		if htmlPath, ok := entry.StaticRoutes[normalizedPath]; ok {
			return PageDecision{Action: ActionServeRouteFile, HTMLPath: htmlPath}
		}
		return PageDecision{Action: ActionNotFound}
	}
	if req.StaticPath != "" {
		return PageDecision{Action: ActionServeStaticFile, StaticPath: req.StaticPath}
	}
	// In production without manifest/static routes, this is a missing asset.
	// In dev mode this function is not called (dev takes the ActionNeedsSetup path).
	if !req.IsDev {
		return PageDecision{Action: ActionNotFound}
	}
	return PageDecision{Action: ActionRenderStaticPrerender}
}

func ResolveRenderPath(isDev bool, ssrPath string, componentPath string) string {
	if isDev {
		return componentPath
	}
	if ssrPath == "" {
		return ""
	}
	return ssrPath
}

func MatchStaticRoute(manifest *Manifest, entryName string, requestPath string) (htmlPath string, found bool) {
	if manifest == nil {
		return "", false
	}

	entry, ok := manifest.Entries[entryName]
	if !ok || entry.StaticRoutes == nil {
		return "", false
	}

	normalizedPath := NormalizePath(requestPath)
	htmlPath, found = entry.StaticRoutes[normalizedPath]
	return htmlPath, found
}
