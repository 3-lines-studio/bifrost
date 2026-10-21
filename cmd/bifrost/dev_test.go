//go:build !windows

package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
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

func TestDevelopmentBinaryGenerationDoesNotOverwriteTheRunningOne(t *testing.T) {
	directory := t.TempDir()
	first := childBinaryPath(directory, 1)
	second := childBinaryPath(directory, 2)
	if first == second {
		t.Fatalf("two generations share the path %s", first)
	}
	if first == filepath.Join(directory, "app") {
		t.Fatal("a generation writes over the binary that is running")
	}
}

func TestDevelopmentLockNamesTheProcessThatHoldsIt(t *testing.T) {
	base := t.TempDir()
	name := "bifrost-dev-1111111111111111"
	directory := filepath.Join(base, "web")

	held, err := claimDevelopmentServer(base, name, directory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()
	if holder := heldBy(filepath.Join(base, name+".lock")); holder == "" {
		t.Fatal("the lock names no process")
	}

	_, err = claimDevelopmentServer(base, name, directory)
	if err == nil {
		t.Fatal("a second run took the lock of the first")
	}
	said := err.Error()
	if !strings.Contains(said, directory) {
		t.Fatalf("the message does not name the directory:\n%s", said)
	}
	if !strings.Contains(said, "pid "+strconv.Itoa(os.Getpid())) {
		t.Fatalf("the message does not name the process that holds it:\n%s", said)
	}
}

func TestDevelopmentStampIgnoresTheFileBifrostWritesForTheApp(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "main.go")
	if err := os.WriteFile(source, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := goTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}

	generated := filepath.Join(root, generatedEmbedSource)
	if err := os.WriteFile(generated, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := goTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("the file bifrost writes is read as a change:\n%s\nwant\n%s", after, before)
	}

	if err := os.WriteFile(filepath.Join(root, "otro.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withSource, err := goTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	if withSource == before {
		t.Fatal("a source somebody wrote is not read as a change")
	}
}

func TestRequireViteInstalledFindsTheModulesOfAnAncestor(t *testing.T) {
	root := t.TempDir()
	modules := filepath.Join(root, "node_modules", "vite")
	if err := os.MkdirAll(modules, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modules, "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "app", "pages")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := requireViteInstalled(nested); err != nil {
		t.Fatalf("the modules of an ancestor were not found: %v", err)
	}
	if err := requireViteInstalled(t.TempDir()); err == nil {
		t.Fatal("a tree without Vite was accepted")
	}
}

func TestPublicTreeStampChangesWithContent(t *testing.T) {
	root := t.TempDir()
	public := filepath.Join(root, "public")
	if err := os.MkdirAll(public, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(public, "style.css")
	if err := os.WriteFile(file, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := publicTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Second)
	if err := os.Chtimes(file, future, future); err != nil {
		t.Fatal(err)
	}
	second, err := publicTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("public stamp did not change: %q", first)
	}
}

func TestTemplateDevelopmentEnvDropsVite(t *testing.T) {
	t.Setenv("BIFROST_DEV_DIR", "/tmp/dev")
	t.Setenv("BIFROST_VITE_PORT", "1234")
	t.Setenv("BIFROST_ADDR", "127.0.0.1:9")
	found := false
	for _, value := range templateDevelopmentEnv() {
		if strings.HasPrefix(value, "BIFROST_DEV_DIR=") || strings.HasPrefix(value, "BIFROST_VITE_") {
			t.Fatalf("environment kept %q", value)
		}
		if value == "BIFROST_ADDR=127.0.0.1:9" {
			found = true
		}
	}
	if !found {
		t.Fatal("BIFROST_ADDR was dropped")
	}
}
