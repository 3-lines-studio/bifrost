package main

import (
	"context"
	"encoding/json"
	"fmt"
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
	// The layout is not in the table because its imports depend on the overlay,
	// and it is still a file the generator writes whole.
	files := append([]nativeFile{{"app/_layout.tsx", nativeLayout("")}, {"bifrost/host.tsx", nativeHost("")}}, nativeProjectFiles...)
	for _, file := range files {
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

func TestNativeProjectDropsOverlayFilesTheOverlayStoppedShipping(t *testing.T) {
	// An app deletes a screen, renames a module, drops a directory, and the file
	// that was copied stays in the project for good: the generator clears `app/`
	// and `bifrost/` and never looks at what the overlay put beside them.
	projectRoot := t.TempDir()
	appRoot := filepath.Join(projectRoot, "app")
	if err := os.MkdirAll(appRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appRoot, "page.tsx"), []byte("export default function Page() { return null; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(projectRoot, "native")
	files := map[string]string{
		"bifrost/session.ts": "export const session = null;\n",
		"components/old.tsx": "export const gone = true;\n",
		"app/index.tsx":      "export default function Page() { return null; }\n",
	}
	for name, content := range files {
		path := filepath.Join(overlay, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prepared, err := prepareAppRouter(t.Context(), projectRoot, appRoot, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(projectRoot, ".bifrost", "native")
	write := func() {
		t.Helper()
		if err := writeNativeProject(native, prepared, "", overlay); err != nil {
			t.Fatal(err)
		}
	}
	write()
	if _, err := os.Stat(filepath.Join(native, "components", "old.tsx")); err != nil {
		t.Fatalf("the overlay file was not copied: %v", err)
	}

	// The overlay drops one file and keeps the other.
	if err := os.Remove(filepath.Join(overlay, "components", "old.tsx")); err != nil {
		t.Fatal(err)
	}
	write()
	if _, err := os.Stat(filepath.Join(native, "components", "old.tsx")); !os.IsNotExist(err) {
		t.Fatalf("a file the overlay no longer ships stayed in the project: %v", err)
	}
	if _, err := os.Stat(filepath.Join(native, "bifrost", "session.ts")); err != nil {
		t.Fatalf("a file the overlay still ships was removed: %v", err)
	}

	// A route is the generator's: an overlay that stops restyling one screen
	// leaves the generated screen in its place, not a hole.
	if err := os.Remove(filepath.Join(overlay, "app", "index.tsx")); err != nil {
		t.Fatal(err)
	}
	write()
	if _, err := os.Stat(filepath.Join(native, "app", "index.tsx")); err != nil {
		t.Fatalf("the generated screen was removed with the overlay's: %v", err)
	}
	// The manifest sits beside the project, not in it: everything inside is a
	// build artifact, and a file there would ship with the app.
	if _, err := os.Stat(overlayManifest(native)); err != nil {
		t.Fatalf("the manifest was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(native, overlayManifestName)); !os.IsNotExist(err) {
		t.Fatalf("the manifest was written into the project: %v", err)
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
	if err := writeNativeProject(native, prepared, "http://example.test", filepath.Join(projectRoot, "native")); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"app/index.tsx", "app/about.tsx", "app/docs/[slug].tsx", "app/_layout.tsx", "bifrost/data.ts", "bifrost/host.tsx", "bifrost/jsx-runtime.ts", "bifrost/metro.js", "bifrost/screen.tsx", "bifrost/origin.ts", "metro.config.js", "app.json"} {
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
	if err := writeNativeProject(native, prepared, "", filepath.Join(projectRoot, "native")); err != nil {
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
	if err := writeNativeProject(native, prepared, "", filepath.Join(projectRoot, "native")); err != nil {
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
	if err := writeNativeProject(native, prepared, server.URL, filepath.Join(projectRoot, "native")); err != nil {
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

func TestNativeProjectKeepsTheAppFilesItWasGiven(t *testing.T) {
	projectRoot := t.TempDir()
	appRoot := filepath.Join(projectRoot, "app")
	if err := os.MkdirAll(appRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appRoot, "page.tsx"), []byte("export default function Page() { return null; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// What an app contributes: a session module of its own, an overridden layout
	// and a client it talks to the server with, plus the dependency it needs.
	overlay := filepath.Join(projectRoot, "native")
	files := map[string]string{
		"bifrost/session.ts": "export const session = null;\n",
		"bifrost/data.ts":    "export const replaced = true;\n",
		"app/_layout.tsx":    "export default function Layout() { return null; }\n",
		"package.json":       `{"dependencies":{"expo-secure-store":"~15.0.7"},"devDependencies":{"prettier":"^3.0.0"},"overrides":{"lightningcss":"1.30.1"},"scripts":{"start":"expo start"}}`,
	}
	for name, content := range files {
		path := filepath.Join(overlay, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	prepared, err := prepareAppRouter(t.Context(), projectRoot, appRoot, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(projectRoot, ".bifrost", "native")
	for run := 1; run <= 2; run++ {
		if err := writeNativeProject(native, prepared, "", overlay); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		for name, want := range files {
			if name == "package.json" {
				continue
			}
			content, err := os.ReadFile(filepath.Join(native, filepath.FromSlash(name)))
			if err != nil {
				t.Fatalf("run %d: %s did not survive: %v", run, name, err)
			}
			if string(content) != want {
				t.Fatalf("run %d: %s was overwritten:\n%s", run, name, content)
			}
		}
	}

	// A generated file nobody overrode stays generated.
	if _, err := os.Stat(filepath.Join(native, "bifrost", "screen.tsx")); err != nil {
		t.Fatalf("the overlay dropped a generated file: %v", err)
	}

	// The manifest keeps what the generator owns and gains what the app declared.
	content, err := os.ReadFile(filepath.Join(native, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Main          string            `json:"main"`
		Dependencies  map[string]string `json:"dependencies"`
		DevDependency map[string]string `json:"devDependencies"`
		Overrides     map[string]string `json:"overrides"`
	}
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatalf("the merged manifest is not valid JSON: %v\n%s", err, content)
	}
	if manifest.Main != "expo-router/entry" {
		t.Fatalf("main = %q", manifest.Main)
	}
	if manifest.Dependencies["expo-secure-store"] != "~15.0.7" {
		t.Fatalf("the app dependency was not merged: %s", content)
	}
	if manifest.Dependencies["expo-router"] == "" {
		t.Fatalf("the generated dependencies were lost: %s", content)
	}
	if manifest.DevDependency["prettier"] != "^3.0.0" {
		t.Fatalf("the app dev dependency was not merged: %s", content)
	}
	// A version pin is the one thing a dependency entry cannot express, because
	// the package manager may hoist another copy that the range also satisfies.
	if manifest.Overrides["lightningcss"] != "1.30.1" {
		t.Fatalf("the app version pin was not merged: %s", content)
	}
}

func TestNativeProjectRefusesAnOverlayRouteTheServerDoesNotServe(t *testing.T) {
	projectRoot := t.TempDir()
	appRoot := filepath.Join(projectRoot, "app")
	if err := os.MkdirAll(appRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appRoot, "page.tsx"), []byte("export default function Page() { return null; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(projectRoot, "native")
	if err := os.MkdirAll(filepath.Join(overlay, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(overlay, "app", "invented.tsx"), []byte("export default function Screen() { return null; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareAppRouter(t.Context(), projectRoot, appRoot, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	err = writeNativeProject(filepath.Join(projectRoot, ".bifrost", "native"), prepared, "", overlay)
	if err == nil || !strings.Contains(err.Error(), "not a route") {
		t.Fatalf("err = %v", err)
	}
}

func TestNativeRuntimeSaysWhatTheTargetCannotPaint(t *testing.T) {
	// A prop whose whole meaning is the document is dropped by the renderer
	// without a word. The measured case is `dangerouslySetInnerHTML`: the body of
	// every message the assistant wrote is markdown compiled to html, and on a
	// native target it painted nothing at all. Nothing in the app says so, the
	// page just comes out missing a paragraph, so the runtime has to.
	dropped, err := nativeTagGroup(nativeHost(""), "DOM_ONLY")
	if err != nil {
		t.Fatal(err)
	}
	if !dropped["dangerouslySetInnerHTML"] {
		t.Fatal("the tag map would drop a leaf of the page in silence")
	}
	// htmlFor is measured noise: fonnso alone writes it twenty-nine times, and
	// dropping it hides no paragraph, so it is not one of them.
	if dropped["htmlFor"] {
		t.Fatal("a label association would warn on every form field")
	}
	if !strings.Contains(nativeHost(""), "export function droppedProps(name: string, props: unknown)") {
		t.Fatal("the tag map does not report the props it cannot honour")
	}
	if !strings.Contains(nativeJSXRuntime, "loaded().droppedProps(name, props)") {
		t.Fatal("the runtime never asks which props are being dropped")
	}
	if !strings.Contains(nativeJSXRuntime, "__DEV__") {
		t.Fatal("the warning would run in production too")
	}
	// The name of the tag is what makes the warning actionable, and the runtime
	// is the last place that still has it.
	if !strings.Contains(nativeJSXRuntime, `warnDropped(type, rest[0])`) {
		t.Fatal("the runtime does not pass the tag name to the warning")
	}
}

func TestNativeWarningsGoInFrontOfTheCompilerTheAppInstalled(t *testing.T) {
	// The compiler collects the values it cannot express and the transformer
	// throws them away, so a class that compiles to nothing looks like one that
	// works. The wrapper has to hand every file to whatever transformer was there
	// and keep its path, the same way the resolver does, or installing it would
	// disable NativeWind instead of reporting what it dropped.
	if !strings.Contains(nativeMetroHelper, "bifrostUpstreamTransformer: config.transformerPath ?? null") {
		t.Fatal("the wrapper does not keep the transformer that was already there")
	}
	if !strings.Contains(nativeCSSWarnings, "config.bifrostUpstreamTransformer ?? config.transformer?.bifrostUpstreamTransformer ?? unstable_transformerPath") {
		t.Fatal("the wrapper does not read the transformer the app installed")
	}
	if !strings.Contains(nativeCSSWarnings, "upstream.transform(config, projectRoot, filePath, data, options)") {
		t.Fatal("the wrapper does not hand the file on")
	}
	if !strings.Contains(nativeCSSWarnings, "compile(css, { ...config.reactNativeCSS, filename: filePath, projectRoot }).warnings()") {
		t.Fatal("the wrapper does not read the warnings it exists for")
	}
	// The warning is only actionable if it names the class the app wrote, which
	// means pairing each dropped value with the selector that asked for it.
	if !strings.Contains(nativeCSSWarnings, "declaration.value.includes(String(value))") {
		t.Fatal("a dropped value is not traced back to its class")
	}
	// The class reaches the stylesheet escaped -- .h-[100dvh] is written
	// .h-\[100dvh\] -- and the app can only search for the name it wrote.
	if !strings.Contains(nativeCSSWarnings, "at: className(selector)") {
		t.Fatal("the report names the selector as the compiler spelled it, not as the app wrote it")
	}
	if !strings.Contains(nativeCSSWarnings, "String.fromCodePoint(parseInt(hex, 16))") {
		t.Fatal("a class Tailwind escaped with a hex escape is reported escaped")
	}
	// An app cannot fix a stylesheet inside a dependency, and a node_modules tree
	// has hundreds of them.
	if !strings.Contains(nativeCSSWarnings, `!filePath.includes("node_modules")`) {
		t.Fatal("the report would list the app's dependencies as its own")
	}
	// Reading the warnings costs a second pass over the stylesheet, which is the
	// web pipeline the native one already ran. A build would pay for a
	// development diagnostic twice.
	if !strings.Contains(nativeCSSWarnings, `options.dev !== false`) {
		t.Fatal("the report runs in a build too")
	}
	if !strings.Contains(nativeCSSWarnings, `options.type !== "asset"`) {
		t.Fatal("a stylesheet asked for as an asset is read as one")
	}
	// Without the compiler there is no stylesheet to drop anything from, and
	// installing a transformer that requires it would break the build.
	if !strings.Contains(nativeMetroHelper, `require.resolve("react-native-css/compiler")`) {
		t.Fatal("the wrapper is installed even when there is no compiler")
	}
	if !strings.Contains(nativeMetroConfig, "withBifrostWarnings(withBifrost(") {
		t.Fatal("the generated config does not install the warnings wrapper")
	}
}

func TestNativeWarningsHandTheFileToTheTransformerTheAppInstalled(t *testing.T) {
	// Metro does not call a transformer with the config the app wrote: it hands it
	// `config.transformer` on its own, flattened, minus the three keys it keeps for
	// itself. Reading the kept path anywhere but the top of that object finds
	// nothing, and the wrapper then falls back to the default transformer, which
	// for a stylesheet on a native target is an empty module. Measured against a
	// real app: the whole stylesheet went out, the app painted unstyled, and
	// nothing in the log said so.
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
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
	if err := writeNativeProject(native, prepared, "", filepath.Join(projectRoot, "native")); err != nil {
		t.Fatal(err)
	}

	// The compiler the helper looks for when it decides whether to install the
	// wrapper, and the two modules the wrapper requires at the top of itself. The
	// stub the wrapper would fall back to answers with a name of its own, so a
	// fallback is a readable failure and not a missing module.
	stub := func(module, source string) {
		dir := filepath.Join(native, "node_modules", module)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "index.js"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"`+module+`","main":"index.js"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stub("react-native-css", "module.exports = { compile: () => ({ warnings: () => ({}) }) };\n")
	if err := os.WriteFile(filepath.Join(native, "node_modules", "react-native-css", "compiler.js"), []byte("module.exports = require(\"./index.js\");\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stub("@expo/metro-config", "module.exports = { unstable_transformerPath: require.resolve(\"./index.js\") };\n")

	probe := `import { createRequire } from "node:module";
const require = createRequire(import.meta.url);
const path = require("node:path");
const fs = require("node:fs");
const { withBifrostWarnings } = require("./bifrost/metro.js");
// What the app wrote, and what its transformer answers with.
const upstream = path.join(process.cwd(), "upstream.js");
fs.writeFileSync(upstream, "module.exports.transform = async () => ({ output: [{ data: { code: \"UPSTREAM\" } }] });\n");
const warned = withBifrostWarnings({ transformerPath: upstream, transformer: { reactNativeCSS: {} } });
// What Metro does with that config before it calls the transformer: the three
// keys it keeps for itself come out, and the rest is the config.
const { getTransformOptions, transformVariants, unstable_workerThreads, ...transformerConfig } = warned.transformer;
const wrapper = require(warned.transformerPath);
wrapper
  .transform(transformerConfig, process.cwd(), path.join(process.cwd(), "global.css"), Buffer.from(""), {
    platform: "android", dev: false, type: "module", minify: false, customTransformOptions: {},
  })
  .then((result) => console.log(JSON.stringify({ code: result.output[0].data.code })));
`
	if err := os.WriteFile(filepath.Join(native, "probe.mjs"), []byte(probe), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, node, "probe.mjs")
	command.Dir = native
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("the warnings wrapper failed: %v: %s", err, output)
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(output))), &got); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	if got.Code != "UPSTREAM" {
		t.Fatalf("the file did not reach the transformer the app installed: it answered %q", got.Code)
	}
}

func TestNativeOverlayThatReplacesTheMetroConfigHasToInstallTheWarnings(t *testing.T) {
	// Everything the generated metro.config.js installs is lost the moment an app
	// replaces that file, and this one is lost in silence: a transformer that was
	// never wrapped reports nothing, which is what a stylesheet with nothing to
	// report looks like too. The generator is the only thing that can see the file
	// and it says so. Measured: withNativewind applied after withBifrostWarnings
	// puts back its own transformer and the warning never runs.
	projectRoot := t.TempDir()
	overlay := filepath.Join(projectRoot, "native")
	if err := os.MkdirAll(overlay, 0o755); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(overlay, "metro.config.js")
	if err := os.WriteFile(name, []byte(`module.exports = withBifrost(getDefaultConfig(__dirname));`), 0o644); err != nil {
		t.Fatal(err)
	}
	warning := nativeOverlayWarning(overlay)
	if warning == "" {
		t.Fatal("an overlay that loses the diagnostics is left alone")
	}
	if !strings.Contains(warning, "withBifrostWarnings") {
		t.Fatal("the warning does not say what is missing")
	}
	// The file it names is the one the app has to open, and the advice is the call
	// that makes the wrapper outermost.
	if !strings.Contains(filepath.ToSlash(warning), "native/metro.config.js") {
		t.Fatal("the warning does not name the file that has to change")
	}
	if !strings.Contains(warning, "withBifrostWarnings(withBifrost(yourConfig))") {
		t.Fatal("the warning does not say how to install it")
	}
	if err := os.WriteFile(name, []byte(`module.exports = withBifrostWarnings(withBifrost(getDefaultConfig(__dirname)));`), 0o644); err != nil {
		t.Fatal(err)
	}
	if warning := nativeOverlayWarning(overlay); warning != "" {
		t.Fatalf("an overlay that installs the wrapper is told it does not: %s", warning)
	}
	// Most overlays have no metro.config.js at all, and they take the generated
	// one, which installs the wrapper by itself.
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	if warning := nativeOverlayWarning(overlay); warning != "" {
		t.Fatalf("an overlay with no metro config is told it is wrong: %s", warning)
	}
}

func TestNativeLayoutCarriesWhatTheOverlayShips(t *testing.T) {
	// The generated layout is the shell an app would otherwise rewrite whole to
	// add one line to it. Measured on fonnso: the file was replaced to import a
	// stylesheet, a module of its own and the network shim, and two of those three
	// lines are ones the generator can write.
	cases := []struct {
		name    string
		files   []string
		imports []string
	}{
		{
			"an overlay with neither",
			nil,
			[]string{`import "../bifrost/net";`},
		},
		{
			"a global stylesheet",
			[]string{"global.css"},
			[]string{`import "../global.css";`, `import "../bifrost/net";`},
		},
		{
			"a module of its own",
			[]string{"bifrost/setup.ts"},
			[]string{`import "../bifrost/setup";`, `import "../bifrost/net";`},
		},
		{
			"both, before the shim that wraps what they install",
			[]string{"global.css", "bifrost/setup.tsx"},
			[]string{`import "../global.css";`, `import "../bifrost/setup";`, `import "../bifrost/net";`},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			projectRoot := t.TempDir()
			overlay := filepath.Join(projectRoot, "native")
			for _, name := range test.files {
				path := filepath.Join(overlay, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			layout := nativeLayout(overlay)
			// The extension is left off: a tsconfig that has not asked for
			// allowImportingTsExtensions rejects an import that spells it out.
			want := strings.Join(test.imports, "\n") + "\n\n"
			if !strings.HasPrefix(layout, want) {
				t.Fatalf("the layout opens with\n%q\nwant\n%q", layout, want)
			}
			if !strings.Contains(layout, "expo-router") || !strings.Contains(layout, "SafeAreaProvider") {
				t.Fatal("the layout is a header with no shell under it")
			}
		})
	}
}

func TestNativeShipsThePiecesEveryClientNeeds(t *testing.T) {
	// Three things every client of a web app needs, and none of them is the app's
	// business: an origin in front of a relative request, a React.createElement
	// that reaches the tag map, and a layout that imports both.
	files := map[string]string{}
	for _, file := range nativeProjectFiles {
		files[file.Name] = file.Content
	}
	net, ok := files["bifrost/net.ts"]
	if !ok {
		t.Fatal("a page that asks its server for /api/... has nothing to put the origin there")
	}
	if !strings.Contains(net, `url.startsWith("/")`) || !strings.Contains(net, "original(ORIGIN + url, init)") {
		t.Fatal("the shim does not do what the browser does to a relative url")
	}
	// A Request and a URL spell their address differently, and only one of them
	// has a url at all.
	if !strings.Contains(net, "input instanceof URL") {
		t.Fatal("a URL object is read as if it had a url")
	}
	react, ok := files["bifrost/react.js"]
	if !ok {
		t.Fatal("a library that builds its elements by hand would paint a tag for the web")
	}
	if !strings.Contains(react, "React.createElement = (type, ...rest) =>") {
		t.Fatal("the patch does not reach React.createElement")
	}
	// A copy of the module holds the own enumerable keys only, and a library that
	// imports React as a namespace would get undefined for the rest.
	if !strings.Contains(react, "module.exports = React") {
		t.Fatal("the patch hands back a copy instead of the module itself")
	}
	if !strings.Contains(nativeLayout(""), `import "../bifrost/net";`) {
		t.Fatal("nothing imports the shim, so every relative request goes nowhere")
	}
}

func TestNativeMetroHelperReadsThePathAliasesOfTheApp(t *testing.T) {
	// Metro has never read TypeScript paths, and the web bundler gets them from a
	// config of its own, so an app that spells its imports "@/components/x" fails
	// on the first one.
	for _, want := range []string{
		`fs.readFileSync(path.join(appRoot, "tsconfig.json"), "utf8")`,
		"options.paths",
		"options.baseUrl",
		"pathAliases(workspaceRoot)",
		"underAlias(alias, moduleName)",
		"path.join(alias.to, under + alias.after)",
	} {
		if !strings.Contains(nativeMetroHelper, want) {
			t.Fatalf("the helper never reads %s", want)
		}
	}
	// A tsconfig is written by an editor, which leaves comments in it and a
	// trailing comma, and a JSON parser refuses both. Measured: the paths of
	// {"paths":{"@/*":["./*"],}} are read with both present.
	if !strings.Contains(nativeMetroHelper, "function asJSON(") {
		t.Fatal("a commented tsconfig would declare no paths at all")
	}
	// A target that is not a directory under the base -- "@/*": ["*"] with a
	// baseUrl -- is a valid mapping, and so is one that puts something after the
	// wildcard.
	if !strings.Contains(nativeMetroHelper, "alias.suffix") {
		t.Fatal("only a pattern that ends in a wildcard would be honoured")
	}
	// The generated project imports the real React, or the patch is installed
	// inside the thing that installs it.
	if !strings.Contains(nativeMetroHelper, `moduleName === "react" && !insideBifrost`) {
		t.Fatal("the react patch would be installed inside the project that installs it")
	}
}

func TestNativeTagMapTakesWhatTheAppShipsOnTopOfIt(t *testing.T) {
	// The tag map is where a tag becomes the component that paints it, and every
	// app has a tag the generator does not know, so every app that needs one used
	// to replace the whole file. Measured, that is not a cost you pay once: an app
	// replaced it, the file later grew a function the runtime calls, the replaced
	// copy did not have it, and every element of the app threw in development.
	projectRoot := t.TempDir()
	overlay := filepath.Join(projectRoot, "native")
	if err := os.MkdirAll(filepath.Join(overlay, "bifrost"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, extension := range []string{".ts", ".tsx", ".js", ".jsx"} {
		if content := nativeHost(overlay); strings.Contains(content, "extraTags") {
			t.Fatalf("the map reads an app module that is not there (%s)", extension)
		}
		if err := os.WriteFile(filepath.Join(overlay, "bifrost", "host-extra"+extension), []byte("export const extraTags = {};\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		content := nativeHost(overlay)
		// The import leaves the extension off, like the layout's: a tsconfig that
		// has not asked for allowImportingTsExtensions rejects one that spells it.
		if !strings.Contains(content, `import { extraTags } from "./host-extra";`) {
			t.Fatalf("the map does not read the app's tags (%s)", extension)
		}
		if !strings.Contains(content, "NAMED[name] = component;") {
			t.Fatalf("the app's tags are never merged into the table (%s)", extension)
		}
		// Merged after the table is built, or a tag the app adds would be the one
		// the table overwrites.
		if strings.Index(content, "NAMED[name] = component;") < strings.Index(content, "for (const name of HIDDEN)") {
			t.Fatalf("the app's tags are merged before the table is built (%s)", extension)
		}
		// Everything the generator puts in the file has to survive the seam: that
		// is the whole reason it exists.
		for _, kept := range []string{
			"export function hostComponent(name: string): unknown",
			"export function droppedProps(name: string, props: unknown)",
			"no native counterpart for",
		} {
			if !strings.Contains(content, kept) {
				t.Fatalf("the seam lost %q (%s)", kept, extension)
			}
		}
		if err := os.Remove(filepath.Join(overlay, "bifrost", "host-extra"+extension)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNativeShipsTheDocumentShimWithoutTurningItOn(t *testing.T) {
	// A page written for the browser reaches for a document from places the tag
	// map cannot cover, and every app that needs one writes the same shim. It is
	// written for every app and imported by none: a module nothing imports is
	// never bundled, and the app turns it on with a line in its own setup.
	files := map[string]string{}
	for _, file := range nativeProjectFiles {
		files[file.Name] = file.Content
	}
	dom, ok := files["bifrost/dom.js"]
	if !ok {
		t.Fatal("an app that touches a document has nothing to turn on")
	}
	for _, said := range []string{
		"target.document ??=",
		"target.localStorage ??=",
		"target.matchMedia ??=",
		"target.ResizeObserver ??= ResizeObserver",
		"target.Node ??=",
		"target.navigator.clipboard",
	} {
		if !strings.Contains(dom, said) {
			t.Fatalf("the shim does not answer %s", said)
		}
	}
	// A shim that lied about the cookie it hands back is worse than one that says
	// there is none.
	if !strings.Contains(dom, "get cookie() {") || !strings.Contains(dom, "return String();") {
		t.Fatal("the shim hands back a cookie that is not there")
	}
	// Nothing of it is installed at the cost of what is already there: every
	// assignment is a fallback, so an app that filled one in keeps its own.
	if strings.Contains(dom, "target.document =") || strings.Contains(dom, "target.window =") {
		t.Fatal("the shim overwrites what the app or the target already installed")
	}
	// The generated layout imports the app's setup and the network shim, and the
	// document shim only when the app asks for it.
	if strings.Contains(nativeLayout(""), "dom") {
		t.Fatal("the document shim is turned on for every app")
	}
}

func TestNativePageTableFollowsTheRouteTable(t *testing.T) {
	// What the app does not own is not a screen the router can paint, and on the
	// web the server answers a url like that -- an api route that redirects to a
	// checkout, say. Telling one from the other takes the table of pages, and the
	// table comes from the same route table as everything else.
	got := nativePagePaths([]string{"index", "admin/settings", "docs/[slug]", "docs/[...rest]"})
	want := []string{"/", "/admin/settings", "/docs/[...rest]", "/docs/[slug]"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the pages of the app are %v, want %v", got, want)
	}
	if source := nativeRoute([]string{"index"}); !strings.Contains(source, `"/"`) {
		t.Fatalf("the page at the root is not a page of the app:\n%s", source)
	}
	if source := nativeRoute(got); !strings.Contains(source, "export function hasPage(url: string): boolean") {
		t.Fatal("nothing can tell a page of the app from anything else")
	}
}

func TestNativePageTableLeavesOutTheNotFoundRoute(t *testing.T) {
	// A not-found route is a catch-all: it is what the server answers a url the
	// app does not have. Measured on the demo app: with it in the table every url
	// -- /api/... included -- was a page, and the shim would have pushed a screen
	// instead of handing the url to the browser, where the server answers it.
	projectRoot := t.TempDir()
	appRoot := filepath.Join(projectRoot, "app")
	for _, name := range []string{"page.tsx", "not-found.tsx", "labs/page.tsx", "labs/not-found.tsx"} {
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
	if err := writeNativeProject(native, prepared, "", filepath.Join(projectRoot, "native")); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(native, "bifrost", "route.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "[...path]") {
		t.Fatalf("a not-found route became a page of the app:\n%s", source)
	}
	for _, page := range []string{`"/"`, `"/labs"`} {
		if !strings.Contains(string(source), page) {
			t.Fatalf("%s is not a page of the app:\n%s", page, source)
		}
	}
}

func TestNativePagesAreMatchedTheWayAUrlSpellsThem(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	if !nodeRunsTypeScript(t, node) {
		t.Skip("node is older than 22.6 and cannot run the generated TypeScript")
	}
	// The table is what decides whether a url the app asks for is a page it can
	// paint or something the server answers. A segment in brackets is one the
	// router fills in, a segment in brackets with three dots takes the rest of the
	// url, and a query or a fragment is none of the table's business.
	projectRoot := t.TempDir()
	appRoot := filepath.Join(projectRoot, "app")
	for _, name := range []string{"page.tsx", "not-found.tsx", "docs/page.tsx", "docs/slug_/page.tsx", "docs/rest__/page.tsx"} {
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
	if err := writeNativeProject(native, prepared, "http://example.test", filepath.Join(projectRoot, "native")); err != nil {
		t.Fatal(err)
	}
	probe := `import { hasPage } from "./bifrost/route.ts";
for (const url of ["/", "/docs", "/docs/one", "/docs/one/two?x=1#y", "/api/stripe/checkout", "/other"]) {
  console.log(url + " " + hasPage(url));
}
`
	if err := os.WriteFile(filepath.Join(native, "probe.mjs"), []byte(probe), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, node, "probe.mjs")
	command.Dir = native
	output, err := command.Output()
	if err != nil {
		t.Fatalf("the generated table failed: %v", err)
	}
	want := []string{
		"/ true",
		"/docs true",
		"/docs/one true",
		"/docs/one/two?x=1#y true",
		// An api route is not a page of the app, and neither is a url the app does
		// not have: the not-found route catches it and is not a page either.
		"/api/stripe/checkout false",
		"/other false",
	}
	if got := strings.Split(strings.TrimSpace(string(output)), "\n"); !reflect.DeepEqual(got, want) {
		t.Fatalf("hasPage answered\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestNativeAddressIsWhereThePageOnTopIs(t *testing.T) {
	// window.location is read by a page whenever it likes, and in a browser the
	// address is there to be read. Here it is a value of the navigation state, so
	// the screen on top writes it down -- while it renders and not in an effect:
	// a page reads the address in its own first effect, and the effect of a child
	// runs before the effect of its parent. Measured on fonnso: onboarding.tsx
	// reads window.location.search in an effect that runs on mount.
	if !strings.Contains(nativeScreen, `import { setAddress } from "./route";`) {
		t.Fatal("the screen does not write down where it is")
	}
	if !strings.Contains(nativeScreen, "if (focused) {\n    setAddress(href);\n  }") {
		t.Fatal("a screen behind this one writes down an address that is not the app's")
	}
	// It has to be written before the screen gives up on rendering and answers
	// with a spinner, or a page that is still loading has no address at all.
	write := strings.Index(nativeScreen, "setAddress(href);")
	if write < 0 || write > strings.Index(nativeScreen, `state.status === "loading"`) {
		t.Fatal("the address is written after the screen stopped rendering the page")
	}
}

func TestNativeShimAnswersWhereThePageIs(t *testing.T) {
	// A page that asks where it is, is at, or was, reaches for window.location and
	// window.history, and neither exists on a native target. The shim answers with
	// the address the screen wrote down, in the shape of the urls of the server
	// that serves the pages, and moves the router for a page of the app.
	files := map[string]string{}
	for _, file := range nativeProjectFiles {
		files[file.Name] = file.Content
	}
	dom, ok := files["bifrost/dom.js"]
	if !ok {
		t.Fatal("an app that reads the address has nothing to turn on")
	}
	for _, said := range []string{
		`const { ORIGIN } = require("./origin");`,
		`const { getAddress, setAddress, hasPage, onAddressChange } = require("./route");`,
		"target.location ??= location;",
		"target.history ??= history;",
		"return ORIGIN + getAddress();",
		"if (page !== null && hasPage(page)) {",
		"setTimeout(() => fire(\"popstate\"), 0);",
	} {
		if !strings.Contains(dom, said) {
			t.Fatalf("the shim does not answer %s", said)
		}
	}
	// A url the app does not own is answered by the server on the web: the browser
	// leaves the document and lands wherever it redirects. That is what the shim
	// hands over, rather than a screen the router cannot fill.
	if !strings.Contains(dom, "Linking().openURL(target)") {
		t.Fatal("a url that is not a page of the app goes into the router")
	}
	if !strings.Contains(dom, "nothing on this device could open") {
		t.Fatal("a url that opened nothing says nothing")
	}
	// A relative url is resolved against the directory of the current one, which
	// is a piece of a browser this does not carry: naming it beats guessing.
	if !strings.Contains(dom, "needs a rooted or absolute url") {
		t.Fatal("a relative url is guessed instead of refused")
	}
	// A page that pushes an entry of its own to catch the back button -- the image
	// viewer of thread.tsx -- is answered with the pop it expects and no move.
	if !strings.Contains(dom, "pushed.push(true);") {
		t.Fatal("an entry a page pushed with no url to move to is dropped")
	}
	// The runtime the app starts in installs a location of its own that answers the
	// url of the bundler, and it cannot be taken over afterwards -- measured, the
	// property refuses an assignment (NotSupportedError) and it is not
	// configurable, so a redefinition is refused too. What is left is the runtime's
	// own switch for not installing one, which the generated app.json carries, and
	// a warning for the day it installs one anyway.
	if !strings.Contains(dom, "if (target.location && target.location !== location) {") {
		t.Fatal("a location that answers where the bundler is would be answered silently")
	}
	if !strings.Contains(dom, "extra.router.origin set to false") {
		t.Fatal("the warning does not name what turns the runtime's location off")
	}
	// Nothing else is installed at the cost of what is already there.
	if strings.Contains(dom, "target.document =") || strings.Contains(dom, "target.window =") {
		t.Fatal("the shim overwrites the document or the window the app installed")
	}
}

func TestNativeAppManifestTurnsOffTheLocationOfTheRuntime(t *testing.T) {
	// The runtime installs a window.location that answers the url of the bundler,
	// and it defines it so that it can be neither assigned nor redefined. Its own
	// switch for not installing one is `extra.router.origin: false`, and the
	// generated app.json is the only place that can turn it off: an app that ships
	// an app.json of its own replaces the whole file, and then this is the first
	// thing to go.
	expo, ok := nativeAppManifest("/tmp/demo")["expo"].(map[string]any)
	if !ok {
		t.Fatal("the app manifest has no expo section")
	}
	extra, ok := expo["extra"].(map[string]any)
	if !ok {
		t.Fatal("the app manifest carries no extra")
	}
	router, ok := extra["router"].(map[string]any)
	if !ok {
		t.Fatal("the app manifest does not speak to the router")
	}
	origin, present := router["origin"]
	if !present || origin != false {
		t.Fatalf("the runtime is left installing a location of its own: origin=%v", origin)
	}

	projectRoot := t.TempDir()
	native := filepath.Join(projectRoot, ".bifrost", "native")
	if err := writeNativeJSON(native, "app.json", nativeAppManifest(projectRoot)); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(filepath.Join(native, "app.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), `"origin": false`) {
		t.Fatalf("the generated app.json does not turn it off:\n%s", written)
	}
}

func TestNativeTagMapCoversWhatAPageUses(t *testing.T) {
	groups := map[string]map[string]bool{}
	for _, name := range []string{"BOXES", "TEXTS", "HIDDEN"} {
		tags, err := nativeTagGroup(nativeHost(""), name)
		if err != nil {
			t.Fatal(err)
		}
		if len(tags) == 0 {
			t.Fatalf("%s is empty", name)
		}
		groups[name] = tags
	}
	for outer, tags := range groups {
		for _, other := range []string{"BOXES", "TEXTS", "HIDDEN"} {
			if other == outer {
				continue
			}
			for tag := range tags {
				if groups[other][tag] {
					t.Fatalf("%q is in both %s and %s", tag, outer, other)
				}
			}
		}
	}

	// The tags the demo pages and fonnso use in a page, and the ones the map
	// answers with a component of its own. Measured by grep over both trees; a
	// tag outside this list and outside the groups renders nothing here.
	wanted := []string{
		"a", "article", "button", "code", "div", "footer", "form", "h1", "h2",
		"h3", "header", "img", "input", "label", "legend", "li", "link", "main",
		"nav", "ol", "p", "pre", "section", "small", "span", "table", "tbody",
		"td", "textarea", "tfoot", "th", "thead", "title", "tr", "ul",
	}
	for _, tag := range wanted {
		if groups["BOXES"][tag] || groups["TEXTS"][tag] || groups["HIDDEN"][tag] {
			continue
		}
		if nativeTagIsNamed(tag) || tag == "hr" {
			continue
		}
		t.Errorf("no native counterpart for <%s>", tag)
	}
}

func TestNativeTagMapDoesNotPretendToPaintAControl(t *testing.T) {
	// A dropdown is behaviour, not a tag: it needs a picker, so the map would
	// have to lie about it. It stays out of the vocabulary on purpose and says so
	// in development, which is how the app learns it needs a native counterpart
	// of its own. fonnso has one, in the primitives a screen is built from.
	for _, name := range []string{"BOXES", "TEXTS", "HIDDEN"} {
		tags, err := nativeTagGroup(nativeHost(""), name)
		if err != nil {
			t.Fatal(err)
		}
		for _, tag := range []string{"select", "option", "optgroup"} {
			if tags[tag] {
				t.Fatalf("<%s> is claimed as %s", tag, name)
			}
		}
	}
}

func TestNativeTagMapSubmitsAFormTheWayABrowserDoes(t *testing.T) {
	// Measured on fonnso: the login page is a <form onSubmit> with a button of
	// type="submit", and pressing that button on the device made no request at
	// all. A Pressable carrying type="submit" is a Pressable that carries a prop
	// nothing reads, and the fields of the form go nowhere.
	host := nativeHost("")
	if !nativeTagIsNamed("form") {
		t.Fatal("<form> has no counterpart of its own")
	}
	boxes, err := nativeTagGroup(host, "BOXES")
	if err != nil {
		t.Fatal(err)
	}
	if boxes["form"] {
		t.Fatal("<form> is a box, so nothing of it survives")
	}
	for _, wanted := range []string{
		// The button asks the form it is inside, and only when it is a submit.
		"usePress(type, onPress as (() => void) | undefined)",
		// The browser's name for the press is answered, not dropped.
		"export function nativeProps(props: unknown): unknown",
		// The form keeps its fields and answers the submit with them, in a module
		// of its own, so that a button the app paints itself still submits.
		"const [fields] = useState(() => new Map<string, string>());",
		"body.append(name, value)",
		// What the page reads out of the event is the FormData the form built.
		"source instanceof NativeFormData",
	} {
		if !strings.Contains(host, wanted) && !strings.Contains(nativeForm, wanted) && !strings.Contains(nativeJSXRuntime, wanted) {
			t.Fatalf("neither the tag map nor the form carries %q", wanted)
		}
	}
}

func TestNativeTagMapKeepsTheValueOfEveryFieldOfAForm(t *testing.T) {
	// A page that reads its fields out of the event -- `new FormData(event
	// .currentTarget).get("email")` -- asks the document for them. There is no
	// document, so the answer has to come from the field itself: what the page
	// set, and what somebody typed into a field the page let keep its own value.
	host := nativeHost("")
	if !strings.Contains(host, "function Field({ name, value, defaultValue, ...rest }") {
		t.Fatal("a field does not know its own name")
	}
	if !strings.Contains(host, `form.fields.set(name, held === undefined || held === null ? "" : String(held))`) {
		t.Fatal("a field the page holds does not reach the form")
	}
	if !strings.Contains(host, "form.fields.set(name, text)") {
		t.Fatal("what somebody typed into a field does not reach the form")
	}
	if !nativeTagIsNamed("input") || !nativeTagIsNamed("textarea") {
		t.Fatal("a field of a form is not in the table")
	}
}

func TestNativeRuntimeLeavesNamesItDoesNotKnowAlone(t *testing.T) {
	// react-native builds its own views out of strings (`codegenNativeComponent`
	// returns "RCTView"), so replacing an unknown name would tear them apart.
	if !strings.Contains(nativeJSXRuntime, "mapped === undefined ? name : mapped") {
		t.Fatal("the runtime does not fall back to the name it was given")
	}
	if !strings.Contains(nativeHost(""), "return undefined;") {
		t.Fatal("the tag map does not report a name it does not know")
	}
	if !strings.Contains(nativeMetroHelper, "originModulePath.startsWith") {
		t.Fatal("the runtime would resolve its own import and recurse")
	}
}

func TestNativeScreensOnlyFetchWhileFocused(t *testing.T) {
	// A stack keeps the screens behind the current one mounted, so a screen that
	// reads global params would fetch the url of whichever route is on top and
	// render that page under a pathname that is no longer its own.
	if strings.Contains(nativeScreen, "useGlobalSearchParams") {
		t.Fatal("the screen reads the global params")
	}
	if !strings.Contains(nativeScreen, "useLocalSearchParams") {
		t.Fatal("the screen does not read its own params")
	}
	if !strings.Contains(nativeScreen, "if (!focused)") {
		t.Fatal("the screen fetches while it is not focused")
	}
	if !strings.Contains(nativeScreen, "[href, nonce, focused]") {
		t.Fatal("the screen does not refetch when it comes back into focus")
	}
}

// nativeTagGroup reads one of the tag lists out of the generated runtime, so the
// test checks the table the app is written against and not a copy of it.
func nativeTagGroup(source, name string) (map[string]bool, error) {
	opening := "const " + name + " = ["
	start := strings.Index(source, opening)
	if start < 0 {
		return nil, fmt.Errorf("the runtime has no %s", name)
	}
	rest := source[start+len(opening):]
	end := strings.Index(rest, "]")
	if end < 0 {
		return nil, fmt.Errorf("%s is not closed", name)
	}
	tags := map[string]bool{}
	for _, quoted := range strings.Split(rest[:end], ",") {
		tag := strings.TrimSpace(quoted)
		if tag == "" {
			continue
		}
		parsed, err := strconv.Unquote(tag)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		tags[parsed] = true
	}
	return tags, nil
}

// nativeTagIsNamed reports whether the map answers a tag with a component of its
// own instead of a group.
func nativeTagIsNamed(tag string) bool {
	return strings.Contains(nativeHost(""), "\n  "+tag+": ")
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

func TestNativeScreenFallsIntoTheErrorViewsTheResponseCounts(t *testing.T) {
	// The generator writes the branch into the view (props.__bifrostError and
	// props.__bifrostErrorLevel), but the server is not the one rendering on a
	// native target, so nothing fills them. Without a boundary around the view
	// the throw reaches whoever mounts the route and the error.tsx of the app is
	// never painted.
	opened := strings.Index(nativeScreen, "<Boundary")
	rendered := strings.Index(nativeScreen, "{render(props, pageKey)}")
	closed := strings.Index(nativeScreen, "</Boundary>")
	if opened < 0 || rendered < 0 || closed < 0 || !(opened < rendered && rendered < closed) {
		t.Fatal("the screen renders the view outside a boundary")
	}
	if !strings.Contains(nativeScreen, "onError={onError}") {
		t.Fatal("the boundary does not hand the failure to the screen")
	}
	if !strings.Contains(nativeScreen, "__bifrostError: fallback.message") ||
		!strings.Contains(nativeScreen, "__bifrostErrorLevel: fallback.level") {
		t.Fatal("the screen does not tell the view which error view it fell into")
	}
	// The deepest error view comes first, and every further throw climbs one
	// level, which is the order the server walks them in.
	if !strings.Contains(nativeScreen, "{ level: (data.errorFallbacks ?? 0) - 1, message }") {
		t.Fatal("the screen does not start at the innermost error view")
	}
	if !strings.Contains(nativeScreen, "{ level: current.level - 1, message }") {
		t.Fatal("the screen does not climb to the next error view")
	}
	// A remount is what makes the boundary try again instead of staying failed.
	if !strings.Contains(nativeScreen, `key={fallback === null ? "page" : "error-" + fallback.level}`) {
		t.Fatal("the boundary is not remounted when the level changes")
	}
}

func TestNativeScreenAppliesTheDocumentAttributes(t *testing.T) {
	// lang, class and dir travel with every page because they were the
	// attributes of the root element of the document. A class is the theme the
	// tree reads and a direction is how the tree is laid out; a language has no
	// counterpart here, because a text node inherits none to read from.
	if !strings.Contains(nativeScreen, "className={document.class}") {
		t.Fatal("the screen drops the document class")
	}
	if !strings.Contains(nativeScreen, "{ direction }") {
		t.Fatal("the screen drops the document direction")
	}
	if !strings.Contains(nativeScreen, `document.dir === "rtl" || document.dir === "ltr"`) {
		t.Fatal("the screen forwards a direction react-native cannot take")
	}
	// The wrapper is only there when there is something to put on it, so a page
	// whose document carries neither is mounted exactly where it was.
	if !strings.Contains(nativeScreen, "if (document.class === undefined && direction === undefined)") {
		t.Fatal("the screen wraps every page whether or not the document says anything")
	}
}

func TestNativeMetroConfigKeepsTheResolverThatWasAlreadyThere(t *testing.T) {
	// Metro gives the last resolver only the default one, so a second resolver
	// that reached for that would drop the first. An app that adds NativeWind, or
	// anything else with a resolver of its own, replaces metro.config.js whole and
	// wraps this helper, so the seams have to survive that.
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
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
	if err := writeNativeProject(native, prepared, "", filepath.Join(projectRoot, "native")); err != nil {
		t.Fatal(err)
	}

	probe := `import { createRequire } from "node:module";
const require = createRequire(import.meta.url);
const { withBifrost } = require("./bifrost/metro.js");
const reached = [];
const earlier = (context, moduleName, platform) => {
  reached.push(moduleName);
  return { type: "sourceFile", filePath: "earlier:" + moduleName };
};
const config = withBifrost({
  projectRoot: process.cwd(),
  watchFolders: [],
  resolver: { nodeModulesPaths: [], resolveRequest: earlier },
});
const context = { originModulePath: "/app/page.tsx", resolveRequest: () => { throw new Error("the default resolver was reached"); } };
const ask = (name) => config.resolver.resolveRequest(context, name, "android").filePath;
console.log(JSON.stringify({
  navigation: ask("virtual:bifrost/navigation"),
  runtime: ask("react/jsx-runtime"),
  other: ask("some-package"),
  reached,
}));
`
	if err := os.WriteFile(filepath.Join(native, "probe.mjs"), []byte(probe), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, node, "probe.mjs")
	command.Dir = native
	output, err := command.Output()
	if err != nil {
		t.Fatalf("the generated metro helper failed: %v", err)
	}
	want := map[string]string{
		"navigation": "bifrost/navigation.tsx",
		"runtime":    "bifrost/jsx-runtime.ts",
		"other":      "earlier:some-package",
	}
	var got struct {
		Navigation string   `json:"navigation"`
		Runtime    string   `json:"runtime"`
		Other      string   `json:"other"`
		Reached    []string `json:"reached"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(output))), &got); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	for name, value := range map[string]string{"navigation": got.Navigation, "runtime": got.Runtime} {
		if !strings.HasSuffix(value, want[name]) {
			t.Fatalf("%s resolved to %q, want one ending in %q", name, value, want[name])
		}
	}
	if got.Other != want["other"] {
		t.Fatalf("an unowned module resolved to %q, want %q", got.Other, want["other"])
	}
	if len(got.Reached) != 1 || got.Reached[0] != "some-package" {
		t.Fatalf("the resolver that was already there was reached for %v", got.Reached)
	}
}

func TestNativeScreenDoesNotReuseTheLoadingViewAsTheDocumentWrapper(t *testing.T) {
	// The loading view and the wrapper are both a View at the root of the screen,
	// so React keeps the instance and the document class arrives after the first
	// render. react-native-css reads that as the theme changing under the tree and
	// remounts every child, which is a warning an app author would chase.
	if !strings.Contains(nativeScreen, `<View key="loading" style={styles.center}>`) {
		t.Fatal("the loading view has no key to keep it apart from the wrapper")
	}
	if !strings.Contains(nativeScreen, `<View key="failed" style={styles.center}>`) {
		t.Fatal("the failed view has no key to keep it apart from the wrapper")
	}
}

func TestNativeAnswersTheInteractionAPageWritesForTheBrowser(t *testing.T) {
	// Measured on the login page of fonnso: the button that opens the link of a
	// mail answered onClick, and on the device nothing happened at all. React
	// Native answers onPress, so a page written for the browser has to be
	// answered here, on both roads an element is built on: jsx, and createElement
	// for what a component of the app builds by hand.
	host := nativeHost("")
	if !strings.Contains(host, "export function nativeProps(props: unknown): unknown") {
		t.Fatal("the tag map does not answer the interaction of the browser")
	}
	if !strings.Contains(host, "onPress: pressed === undefined ? click : () => { pressed(); click(); }") {
		t.Fatal("a page that answers both names loses one of them")
	}
	if !strings.Contains(nativeJSXRuntime, "loaded().nativeProps(rest[0])") {
		t.Fatal("jsx does not go through it")
	}
	if !strings.Contains(nativeReact, "host.nativeProps(props)") {
		t.Fatal("createElement does not go through it")
	}
	if !strings.Contains(nativeReact, "return createElement(type, ...rest);") {
		t.Fatal("react-native's own views would be rewritten")
	}
}
