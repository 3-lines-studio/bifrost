package core

// PageArtifacts is the resolved client asset description for one page entry (scripts,
// styles, chunks, SSR bundle URL in the manifest). It is the single source of truth for
// HTML shell assembly after resolution from a manifest or dev conventions.
type PageArtifacts struct {
	Script      string
	CriticalCSS string
	CSS         string
	CSSFiles    []string
	Chunks      []string
	IsStatic    bool
	SSRPath     string
}

// ResolvePageArtifacts returns asset metadata for entryName.
// If manifest is nil or the entry has no script, uses the dev/public URL convention:
// /dist/{entryName}.js and /dist/{entryName}.css.
func ResolvePageArtifacts(manifest *Manifest, entryName string) PageArtifacts {
	if manifest != nil {
		if entry, ok := manifest.Entries[entryName]; ok && entry.Script != "" {
			return PageArtifacts{
				Script:      entry.Script,
				CriticalCSS: entry.CriticalCSS,
				CSS:         entry.CSS,
				CSSFiles:    entry.CSSFiles,
				Chunks:      entry.Chunks,
				IsStatic:    entry.Static,
				SSRPath:     entry.SSR,
			}
		}
	}
	return PageArtifacts{
		Script: "/dist/" + entryName + ".js",
		CSS:    "/dist/" + entryName + ".css",
	}
}

// StylesheetHrefsFor returns link hrefs for stylesheets, deduped in order.
func StylesheetHrefsFor(a PageArtifacts) []string {
	return StylesheetHrefs(a.CSS, a.CSSFiles)
}
