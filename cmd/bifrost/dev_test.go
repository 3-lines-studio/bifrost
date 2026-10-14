//go:build !windows

package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestPruneStaleDevelopmentFilesRemovesUnlockedRuns(t *testing.T) {
	base := t.TempDir()
	stale := "bifrost-dev-1111111111111111"
	live := "bifrost-dev-2222222222222222"
	current := "bifrost-dev-3333333333333333"
	orphan := "bifrost-dev-4444444444444444"
	for _, root := range []string{filepath.Join(base, stale+"-1234"), filepath.Join(base, orphan)} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(base, stale+".lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	liveLock, err := os.OpenFile(filepath.Join(base, live+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = liveLock.Close() }()
	if err := syscall.Flock(int(liveLock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}

	pruneStaleDevelopmentFiles(base, current+".lock")

	for _, removed := range []string{filepath.Join(base, stale+".lock"), filepath.Join(base, stale+"-1234")} {
		if _, err := os.Stat(removed); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("%s survived: %v", removed, err)
		}
	}
	for _, kept := range []string{filepath.Join(base, live+".lock"), filepath.Join(base, orphan)} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("%s was removed: %v", kept, err)
		}
	}
}

// writeRoute lays down a page the router will find.
func writeRoute(t *testing.T, routeRoot, name string) {
	t.Helper()
	path := filepath.Join(routeRoot, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("export default function Page() { return null; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDevelopmentStampNoticesARouteArrivingAndLeaving(t *testing.T) {
	// Measured: with only the Go sources watched, a page saved under the route
	// root was never a route -- no rebuild, and a 404 -- until the server was
	// restarted, and deleting the page left the views pointing at a file that was
	// no longer there.
	root := t.TempDir()
	writeRoute(t, root, "page.tsx")
	before, err := routeTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(before, "page.tsx") {
		t.Fatalf("the page of the root is not part of the tree:\n%s", before)
	}

	writeRoute(t, root, "otra/page.tsx")
	added, err := routeTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	if added == before {
		t.Fatal("a page that arrived does not change the route tree")
	}

	writeRoute(t, root, "docs/slug_/page.tsx")
	layout := filepath.Join(root, "layout.tsx")
	if err := os.WriteFile(layout, []byte("export default function Layout() { return null; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	full, err := routeTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(full, "docs/slug_/page.tsx") {
		t.Fatalf("a nested route is missing:\n%s", full)
	}

	if err := os.RemoveAll(filepath.Join(root, "otra")); err != nil {
		t.Fatal(err)
	}
	removed, err := routeTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	if removed == full {
		t.Fatal("a page that left does not change the route tree")
	}
	if strings.Contains(removed, "otra") {
		t.Fatalf("the page that left is still in the tree:\n%s", removed)
	}
}

func TestDevelopmentStampLeavesWhatIsInsideAPageAlone(t *testing.T) {
	// A page is a module of the frontend: Vite hands the new one to the browser
	// and the Go side is not rebuilt for it. What the stamp has to notice is which
	// files are there, not what they say.
	root := t.TempDir()
	writeRoute(t, root, "page.tsx")
	before, err := routeTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(root, "page.tsx")
	if err := os.WriteFile(page, []byte("export default function Page() { return 1; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := routeTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("editing a page changed the route tree:\n%s\nwant\n%s", after, before)
	}

	// A private directory is not a route, the way the router reads it, so a
	// component beside a page is not a route appearing.
	private := filepath.Join(root, "_components")
	if err := os.MkdirAll(private, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(private, "boton.tsx"), []byte("export const Boton = null;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withPrivate, err := routeTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	if withPrivate != before {
		t.Fatalf("a private directory is read as a route:\n%s", withPrivate)
	}
}

func TestDevelopmentStampKeepsWatchingTheGoTree(t *testing.T) {
	// The route tree is what was added; the Go sources are what was watched
	// before, and a server that stops rebuilding them is a server that stopped
	// compiling.
	root := t.TempDir()
	routeRoot := filepath.Join(root, "app")
	writeRoute(t, routeRoot, "page.tsx")
	source := filepath.Join(root, "app.go")

	before, err := developmentStamp(root, routeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withSource, err := developmentStamp(root, routeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if withSource == before {
		t.Fatal("a Go source that arrived does not change the stamp")
	}
}

func TestDevelopmentReloadRegeneratesTheViewsOfTheRouteTree(t *testing.T) {
	// The failure this exists for: a route was removed, the views generated at
	// startup kept pointing at the page that was gone, and every later rebuild
	// failed on it -- "Could not resolve .../app/probe/page.tsx" -- with nothing
	// regenerating them, so the development server stayed dead until it was
	// killed.
	projectRoot := t.TempDir()
	routeRoot := filepath.Join(projectRoot, "app")
	writeRoute(t, routeRoot, "page.tsx")
	writeRoute(t, routeRoot, "probe/page.tsx")

	if _, err := prepareAppRouter(t.Context(), projectRoot, routeRoot, 0, 0); err != nil {
		t.Fatal(err)
	}
	views := filepath.Join(projectRoot, ".bifrost", "views")
	if !viewsMention(t, views, "app/probe/page.tsx") {
		t.Fatal("the view of the route that exists was not written")
	}

	if err := os.RemoveAll(filepath.Join(routeRoot, "probe")); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareAppRouter(t.Context(), projectRoot, routeRoot, 0, 0); err != nil {
		t.Fatalf("reading the route tree again failed: %v", err)
	}
	if viewsMention(t, views, "app/probe/page.tsx") {
		t.Fatal("a view still points at the page that was removed")
	}
}

// viewsMention reports whether any generated view still imports target.
func viewsMention(t *testing.T, views, target string) bool {
	t.Helper()
	entries, err := os.ReadDir(views)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(views, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(content), target) {
			return true
		}
	}
	return false
}
