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

	if !req.IsDev {
		return decideProductionAction(req, entry)
	}

	needsSetup := !IsStaticMode(req.Mode)
	if IsStaticMode(req.Mode) && req.HasRenderer {
		needsSetup = true
	}

	if needsSetup {
		return PageDecision{Action: ActionNeedsSetup, NeedsSetup: true}
	}

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
	if req.HasManifest && entry != nil {
		if htmlPath, ok := LookupStaticRoute(entry, normalizedPath); ok {
			return PageDecision{Action: ActionServeRouteFile, HTMLPath: htmlPath}
		}
		if entry.StaticRoutes != nil {
			return PageDecision{Action: ActionNotFound}
		}
	}
	if req.StaticPath != "" {
		return PageDecision{Action: ActionServeStaticFile, StaticPath: req.StaticPath}
	}
	// Production: no prerendered asset for this request.
	return PageDecision{Action: ActionNotFound}
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

// LookupStaticRoute returns the prerendered HTML path for a normalized URL path.
func LookupStaticRoute(entry *ManifestEntry, normalizedPath string) (htmlPath string, ok bool) {
	if entry == nil || entry.StaticRoutes == nil {
		return "", false
	}
	htmlPath, ok = entry.StaticRoutes[normalizedPath]
	return htmlPath, ok
}

func MatchStaticRoute(manifest *Manifest, entryName string, requestPath string) (htmlPath string, found bool) {
	if manifest == nil {
		return "", false
	}
	entry, ok := manifest.Entries[entryName]
	if !ok {
		return "", false
	}
	return LookupStaticRoute(&entry, NormalizePath(requestPath))
}
