package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDiscoverConventionRoutes(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"page.tsx", "about/page.tsx", "posts/slug_/page.tsx"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	routes, err := discoverConventionRoutes(root)
	if err != nil {
		t.Fatal(err)
	}
	var patterns []string
	for _, route := range routes {
		patterns = append(patterns, route.Pattern)
	}
	if got := strings.Join(patterns, ","); got != "/about,/posts/{slug},/{$}" {
		t.Fatalf("patterns = %q", got)
	}
}

func TestConventionRoots(t *testing.T) {
	projectRoot := t.TempDir()
	appRoot := filepath.Join(projectRoot, "app")
	if err := os.Mkdir(appRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appRoot, "page.tsx"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	project, routes, ok, err := conventionRoots(".", projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || project != projectRoot || routes != appRoot {
		t.Fatalf("roots = %q, %q, %t", project, routes, ok)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "page.tsx"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := conventionRoots(".", projectRoot); err == nil {
		t.Fatal("ambiguous route roots were accepted")
	}
}

func TestNestedConventionViewUsesProjectRelativePath(t *testing.T) {
	projectRoot := t.TempDir()
	routeRoot := filepath.Join(projectRoot, "app")
	if err := os.Mkdir(routeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(routeRoot, "page.tsx"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	routes, err := discoverConventionRoutes(routeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeConventionViews(projectRoot, routeRoot, routes); err != nil {
		t.Fatal(err)
	}
	if routes[0].View != ".bifrost/views/page-0.tsx" {
		t.Fatalf("view = %q", routes[0].View)
	}
	source, err := os.ReadFile(filepath.Join(projectRoot, routes[0].View))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), filepath.Join(routeRoot, "page.tsx")) {
		t.Fatalf("wrapper = %s", source)
	}
}

func TestConventionPatternRejectsInvalidDynamicSegment(t *testing.T) {
	if _, err := conventionPattern("posts/_"); err == nil {
		t.Fatal("empty dynamic segment was accepted")
	}
}

func TestConventionLayoutsComposeOuterToInner(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"layout.tsx":                   "export function Layout({ children }) { return children }",
		"dashboard/layout.tsx":         "export function Layout({ children }) { return children }",
		"dashboard/settings/page.tsx":  "export function Head() { return null } export function Page() { return null }",
		"dashboard/settings/error.tsx": "export function Error() { return null }",
		"dashboard/not-found.tsx":      "export function NotFound() { return null }",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	routes, err := discoverConventionRoutes(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeConventionViews(root, root, routes); err != nil {
		t.Fatal(err)
	}
	view, err := os.ReadFile(filepath.Join(root, ".bifrost", "views", "page-0.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(view)
	if !strings.Contains(text, "export function renderPage") || !strings.Contains(text, `"use no memo";`) {
		t.Fatalf("generated tree factory is not hook-free:\n%s", text)
	}
	if !strings.Contains(text, "pageKey?: string") || !strings.Contains(text, "<Fragment key={pageKey}>") {
		t.Fatalf("generated page branch is not keyed:\n%s", text)
	}
	if !strings.Contains(text, `<Layout0 key={"layout.tsx"}><Layout1 key={"dashboard/layout.tsx"}>`) || !strings.Contains(text, "props.__bifrostError") || !strings.Contains(text, "props.__bifrostNotFound") || !strings.Contains(text, "export { Head }") {
		t.Fatalf("generated view is incomplete:\n%s", text)
	}
}

func TestGeneratedModuleRequiresUserModule(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, ".bifrost", "app")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeConventionModule(generated, root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(generated, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "require example.com/app v0.0.0") || !strings.Contains(text, "replace example.com/app => "+strconv.Quote(filepath.ToSlash(root))) {
		t.Fatalf("generated module does not reference the user module:\n%s", text)
	}
}

func TestGeneratedServerLifecycleAndEscapeHatch(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, ".bifrost", "app")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	routes := []conventionRoute{{Pattern: "/{$}", View: "page.tsx"}}
	goDirs := []conventionGoDir{{Directory: ".", ImportPath: "example.com/app", Alias: "route0", Serve: true}}
	if err := writeConventionMain(root, generated, routes, goDirs); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(generated, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, expected := range []string{"signal.NotifyContext", "ReadHeaderTimeout", "BIFROST_ADDR", `flag.StringVar(&addr, "addr"`, "route0.Serve(ctx, mux)", "server.Shutdown"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("generated main does not contain %q", expected)
		}
	}
	if strings.Contains(text, "WriteTimeout") {
		t.Fatal("generated server sets WriteTimeout")
	}
}

func TestDirectoryContains(t *testing.T) {
	if !directoryContains(".", "dashboard/settings") || !directoryContains("dashboard", "dashboard/settings") || directoryContains("dash", "dashboard") {
		t.Fatal("unexpected middleware ancestry")
	}
}
