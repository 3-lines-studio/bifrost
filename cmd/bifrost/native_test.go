package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
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

func TestNativeClientHandsBackTheRedirectItGets(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	if !nodeRunsTypeScript(t, node) {
		t.Skip("node is older than 22.6 and cannot run the generated TypeScript")
	}

	// A session middleware redirects before the route runs, so a client that
	// follows it silently would render the login props under the path it asked
	// for. The open route covers the other half: the url must not look moved when
	// it is not, or the router is sent in a circle.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/guarded" {
			w.Header().Set("Location", "/login?next=/guarded")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.bifrost.data+json")
		_, _ = w.Write([]byte(`{"pattern":"` + request.URL.Path + `","props":{},"document":{},"status":200}`))
	}))
	defer server.Close()

	projectRoot := t.TempDir()
	appRoot := filepath.Join(projectRoot, "app")
	if err := os.MkdirAll(appRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appRoot, "page.tsx"), []byte("export default function Page() { return null; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareAppRouter(t.Context(), projectRoot, appRoot, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(projectRoot, ".bifrost", "native")
	if err := writeNativeProject(native, prepared, server.URL); err != nil {
		t.Fatal(err)
	}

	// The generated project gets its modules from the install that follows it,
	// and node needs the extension the bundler infers.
	for name, content := range map[string]string{
		"node_modules/react-native/package.json": `{"name":"react-native","version":"0.86.3","main":"index.js"}`,
		"node_modules/react-native/index.js":     "exports.Platform = { OS: \"android\" };\n",
		"bifrost/origin":                         "export * from \"./origin.ts\";\n",
		"probe.mjs":                              "import { loadPage } from \"./bifrost/data.ts\";\nconsole.log(JSON.stringify(await loadPage(\"/guarded\")));\nconsole.log(JSON.stringify(await loadPage(\"/open\")));\n",
	} {
		path := filepath.Join(native, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, node, "probe.mjs")
	command.Dir = native
	output, err := command.Output()
	if err != nil {
		t.Fatalf("the generated client failed: %v", err)
	}
	want := `{"kind":"redirect","href":"/login?next=/guarded"}` + "\n" +
		`{"kind":"page","data":{"pattern":"/open","props":{},"document":{},"status":200}}`
	if got := strings.TrimSpace(string(output)); got != want {
		t.Fatalf("loadPage answered\n%s\nwant\n%s", got, want)
	}
}

func nodeRunsTypeScript(t *testing.T, node string) bool {
	t.Helper()
	output, err := exec.Command(node, "--version").Output()
	if err != nil {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(string(output)), "v"), ".")
	if len(parts) < 2 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	return major > 22 || major == 22 && minor >= 6
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
