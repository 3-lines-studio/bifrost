package main

import (
	"os"
	"path/filepath"
	"slices"
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
	if _, _, err := conventionPattern("posts/_"); err == nil {
		t.Fatal("empty dynamic segment was accepted")
	}
}

func TestConventionPatternNextSyntax(t *testing.T) {
	cases := []struct {
		relative string
		pattern  string
		params   []conventionParam
	}{
		{"about", "/about", nil},
		{"posts/slug_", "/posts/{slug}", []conventionParam{{Name: "slug", Value: "slug"}}},
		{"posts/post-id_", "/posts/{post_id}", []conventionParam{{Name: "post-id", Value: "post_id"}}},
		{"docs/slug__", "/docs/{slug...}", []conventionParam{{Name: "slug", Value: "slug"}}},
		{"marketing~/about", "/about", nil},
		{"marketing~", "/{$}", nil},
	}
	for _, test := range cases {
		pattern, params, err := conventionPattern(test.relative)
		if err != nil {
			t.Fatalf("%s: %v", test.relative, err)
		}
		if pattern != test.pattern {
			t.Fatalf("%s: pattern = %q, want %q", test.relative, pattern, test.pattern)
		}
		if !slices.Equal(params, test.params) {
			t.Fatalf("%s: params = %v, want %v", test.relative, params, test.params)
		}
	}
}

func TestConventionPatternRejectsNextOnlySyntax(t *testing.T) {
	cases := map[string]string{
		"posts/[slug]":    "slug_",
		"posts/[...slug]": "slug__",
		"(marketing)":     "marketing~",
		"@modal":          "parallel route folders are not supported",
		"posts/slug___":   "one _ for a parameter",
		"slug__/posts":    "must be the last segment",
		"posts/post id":   "invalid character",
	}
	for relative, expected := range cases {
		_, _, err := conventionPattern(filepath.FromSlash(relative))
		if err == nil {
			t.Fatalf("%s: invalid segment was accepted", relative)
		}
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("%s: error = %q, want %q", relative, err, expected)
		}
	}
}

func TestConventionPrivateDirectoriesAreNotRouted(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"page.tsx", "_components/Card.tsx"} {
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
	if len(routes) != 1 || routes[0].Pattern != "/{$}" {
		t.Fatalf("private directory was routed: %v", routes)
	}
	path := filepath.Join(root, "_components", "page.tsx")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := discoverConventionRoutes(root); err == nil {
		t.Fatal("page.tsx inside a private directory was accepted")
	}
}

func TestConventionViewsAcceptDefaultExports(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"page.tsx":           "export default function Page() { return null }",
		"layout.tsx":         "export default function Layout({ children }) { return children }",
		"error.tsx":          "export default function Error() { return null }",
		"not-found.tsx":      "export default function NotFound() { return null }",
		"dashboard/page.tsx": "export function Page() { return null }",
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
	entries, err := os.ReadDir(filepath.Join(root, ".bifrost", "views"))
	if err != nil {
		t.Fatal(err)
	}
	var generated strings.Builder
	for _, entry := range entries {
		source, err := os.ReadFile(filepath.Join(root, ".bifrost", "views", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		generated.Write(source)
	}
	for _, expected := range []string{
		"import RoutePage from " + strconv.Quote(filepath.Join(root, "page.tsx")) + ";",
		"import Layout0 from " + strconv.Quote(filepath.Join(root, "layout.tsx")) + ";",
		"import ErrorPage0 from " + strconv.Quote(filepath.Join(root, "error.tsx")) + ";",
		"import NotFound from " + strconv.Quote(filepath.Join(root, "not-found.tsx")) + ";",
		"import { Page as RoutePage } from " + strconv.Quote(filepath.Join(root, "dashboard", "page.tsx")) + ";",
	} {
		if !strings.Contains(generated.String(), expected) {
			t.Fatalf("generated views do not contain %q:\n%s", expected, generated.String())
		}
	}
}

func TestConventionNotFoundRoutesUseTheURLPrefix(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"posts/not-found.tsx", "marketing~/not-found.tsx", "posts/api/not-found.tsx"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	routes, err := appendNotFoundRoutes(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	var patterns []string
	for _, route := range routes {
		patterns = append(patterns, route.Pattern)
	}
	if got := strings.Join(patterns, ","); got != "/posts/api/{path...},/posts/{path...},/{path...}" {
		t.Fatalf("patterns = %q", got)
	}
}

func TestGeneratedMainRenamesPathValuesForGo(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, ".bifrost", "app")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	routes := []conventionRoute{{Pattern: "/posts/{post_id}", Params: []conventionParam{{Name: "post-id", Value: "post_id"}}, View: "page.tsx"}}
	if err := writeConventionMain(root, generated, routes, nil); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(generated, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), `r.SetPathValue("post-id", r.PathValue("post_id"))`) {
		t.Fatalf("generated main does not rename the path value:\n%s", source)
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
	for _, expected := range []string{"signal.NotifyContext", "ReadHeaderTimeout", "BIFROST_ADDR", `flag.StringVar(&addr, "addr"`, "route0.Serve(ctx, app.ResolveMarkdown(mux))", "server.Shutdown"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("generated main does not contain %q", expected)
		}
	}
	if strings.Contains(text, "WriteTimeout") {
		t.Fatal("generated server sets WriteTimeout")
	}
	goDirs[0].Serve = false
	if err := writeConventionMain(root, generated, routes, goDirs); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(generated, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "return serve(ctx, app.ResolveMarkdown(mux))") {
		t.Fatal("generated main does not apply markdown resolution")
	}
}

func TestDirectoryContains(t *testing.T) {
	if !directoryContains(".", "dashboard/settings") || !directoryContains("dashboard", "dashboard/settings") || directoryContains("dash", "dashboard") {
		t.Fatal("unexpected middleware ancestry")
	}
}
