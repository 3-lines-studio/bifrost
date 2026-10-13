package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExpoRouterPathFollowsTheServeMuxPattern(t *testing.T) {
	for pattern, want := range map[string]string{
		"/{$}":            "index",
		"/about":          "about",
		"/docs/{slug}":    "docs/[slug]",
		"/docs/{slug...}": "docs/[...slug]",
		"/{path...}":      "[...path]",
		"/admin/settings": "admin/settings",
	} {
		got, err := expoRouterPath(pattern)
		if err != nil {
			t.Fatalf("%s: %v", pattern, err)
		}
		if got != want {
			t.Fatalf("expoRouterPath(%q) = %q, want %q", pattern, got, want)
		}
	}
}

func TestNativeProjectFilesAreWellFormed(t *testing.T) {
	for _, file := range nativeProjectFiles {
		if filepath.Ext(file.Name) == ".json" {
			var value map[string]any
			if err := json.Unmarshal([]byte(file.Content), &value); err != nil {
				t.Fatalf("%s is not valid JSON: %v", file.Name, err)
			}
			continue
		}
		if strings.Count(file.Content, "{") != strings.Count(file.Content, "}") {
			t.Fatalf("%s has unbalanced braces", file.Name)
		}
	}
}

func TestNativeProjectMirrorsTheRouteTable(t *testing.T) {
	projectRoot := t.TempDir()
	appRoot := filepath.Join(projectRoot, "app")
	for _, name := range []string{"page.tsx", "about/page.tsx", "docs/slug_/page.tsx"} {
		path := filepath.Join(appRoot, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("export default function Page() { return null; }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prepared, err := prepareAppRouter(t.Context(), projectRoot, appRoot, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(projectRoot, ".bifrost", "native")
	if err := writeNativeProject(native, prepared, "http://example.test"); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"app/index.tsx", "app/about.tsx", "app/docs/[slug].tsx", "app/_layout.tsx", "bifrost/screen.tsx", "bifrost/origin.ts", "metro.config.js", "app.json"} {
		if _, err := os.Stat(filepath.Join(native, filepath.FromSlash(name))); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	source, err := os.ReadFile(filepath.Join(native, "app", "docs", "[slug].tsx"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), `params={["slug"]}`) {
		t.Fatalf("dynamic route does not carry its parameters:\n%s", source)
	}
	viewport, err := os.ReadFile(filepath.Join(native, "bifrost", "origin.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(viewport), `"http://example.test"`) {
		t.Fatalf("origin was not written:\n%s", viewport)
	}
}

func TestNativeProjectDropsStaleRoutes(t *testing.T) {
	projectRoot := t.TempDir()
	appRoot := filepath.Join(projectRoot, "app")
	if err := os.MkdirAll(filepath.Join(appRoot, "gone"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"page.tsx", "gone/page.tsx"} {
		if err := os.WriteFile(filepath.Join(appRoot, filepath.FromSlash(name)), []byte("export default function Page() { return null; }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prepared, err := prepareAppRouter(t.Context(), projectRoot, appRoot, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(projectRoot, ".bifrost", "native")
	if err := writeNativeProject(native, prepared, ""); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(native, "app", "gone.tsx")
	if _, err := os.Stat(stale); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(appRoot, "gone")); err != nil {
		t.Fatal(err)
	}
	prepared, err = prepareAppRouter(t.Context(), projectRoot, appRoot, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeNativeProject(native, prepared, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("a removed route left its screen behind: %v", err)
	}
	if _, err := os.Stat(filepath.Join(native, "bifrost", "screen.tsx")); err != nil {
		t.Fatalf("regenerating dropped the shared files: %v", err)
	}
}

func TestNativeNamesFromProjectDirectory(t *testing.T) {
	for base, want := range map[string][2]string{
		"my-app": {"my-app", "my-app"},
		"My App": {"My App", "my-app"},
		"--":     {"--", "bifrost-app"},
	} {
		name, slug := nativeNames(filepath.Join("/tmp", base))
		if got := [2]string{name, slug}; !reflect.DeepEqual(got, want) {
			t.Fatalf("nativeNames(%q) = %v, want %v", base, got, want)
		}
	}
}
