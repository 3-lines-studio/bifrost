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
		switch req.Mode {
		case ModeClientOnly:
			// Check for HTML field in manifest first (new build system)
			if entry != nil && entry.HTML != "" {
				return PageDecision{Action: ActionServeStaticFile, StaticPath: entry.HTML}
			}
			// Fall back to StaticPath (backward compatibility)
			if req.StaticPath != "" {
				return PageDecision{Action: ActionServeStaticFile, StaticPath: req.StaticPath}
			}
		case ModeStaticPrerender:
			if req.HasManifest && entry != nil && entry.StaticRoutes != nil {
				normalizedPath := NormalizePath(req.RequestPath)
				if htmlPath, ok := entry.StaticRoutes[normalizedPath]; ok {
					return PageDecision{Action: ActionServeRouteFile, HTMLPath: htmlPath}
				}
				return PageDecision{Action: ActionNotFound}
			}
			if req.StaticPath != "" {
				return PageDecision{Action: ActionServeStaticFile, StaticPath: req.StaticPath}
			}
		}
	}

	needsSetup := (!req.HasManifest || req.IsDev) && (req.Mode != ModeClientOnly && req.Mode != ModeStaticPrerender)
	if (req.Mode == ModeClientOnly || req.Mode == ModeStaticPrerender) && req.IsDev && req.HasRenderer {
		needsSetup = true
	}

	if needsSetup {
		return PageDecision{Action: ActionNeedsSetup, NeedsSetup: true}
	}

	if req.IsDev && req.Mode == ModeClientOnly {
		return PageDecision{Action: ActionRenderClientOnlyShell}
	}

	if req.IsDev && req.Mode == ModeStaticPrerender {
		return PageDecision{Action: ActionRenderStaticPrerender}
	}

	return PageDecision{Action: ActionRenderSSR}
}

type RenderPathInput struct {
	IsDev         bool
	SSRPath       string
	ComponentPath string
}

func ResolveRenderPath(input RenderPathInput) (string, error) {
	if input.IsDev {
		return input.ComponentPath, nil
	}

	if input.SSRPath == "" {
		return "", nil
	}

	return input.SSRPath, nil
}

func ShouldTriggerSetup(manifest *Manifest, isDev bool, mode PageMode, hasRenderer bool) bool {
	needsSetup := (manifest == nil || isDev) && (mode != ModeClientOnly && mode != ModeStaticPrerender)
	if (mode == ModeClientOnly || mode == ModeStaticPrerender) && isDev && hasRenderer {
		needsSetup = true
	}
	return needsSetup
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
