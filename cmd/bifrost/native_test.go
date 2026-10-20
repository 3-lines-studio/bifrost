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
	"regexp"
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
		if _, err := writeNativeProject(native, prepared, "", overlay); err != nil {
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
	if _, err := writeNativeProject(native, prepared, "http://example.test", filepath.Join(projectRoot, "native")); err != nil {
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
	if _, err := writeNativeProject(native, prepared, "", filepath.Join(projectRoot, "native")); err != nil {
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
	if _, err := writeNativeProject(native, prepared, "", filepath.Join(projectRoot, "native")); err != nil {
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
	if _, err := writeNativeProject(native, prepared, server.URL, filepath.Join(projectRoot, "native")); err != nil {
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
		if _, err := writeNativeProject(native, prepared, "", overlay); err != nil {
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
	_, err = writeNativeProject(filepath.Join(projectRoot, ".bifrost", "native"), prepared, "", overlay)
	if err == nil || !strings.Contains(err.Error(), "not a route") {
		t.Fatalf("err = %v", err)
	}
}

func TestNativeRuntimeSaysWhatTheTargetCannotPaint(t *testing.T) {
	// A prop whose whole meaning is the document is dropped by the renderer
	// without a word, and nothing in the app says so: the page just comes out
	// missing a surface. The one that is left is contentEditable, which is dead
	// text here, and the runtime names it in development.
	dropped, err := nativeTagGroup(nativeHost(""), "DOM_ONLY")
	if err != nil {
		t.Fatal(err)
	}
	if !dropped["contentEditable"] {
		t.Fatal("a surface the target does not have is given up in silence")
	}
	// htmlFor is measured noise: fonnso alone writes it twenty-nine times, and
	// dropping it hides no paragraph, so it is not one of them.
	if dropped["htmlFor"] {
		t.Fatal("a label association would warn on every form field")
	}
	// dangerouslySetInnerHTML was on this list, and it is not any more: it is the
	// body of what a page says. Measured on fonnso, the bubble of every message of
	// the assistant came out empty, twenty-four warnings on one screen. It is
	// painted now, so a warning about it would be a lie.
	if dropped["dangerouslySetInnerHTML"] {
		t.Fatal("the body of a page is reported as dropped while it is painted")
	}
	if !strings.Contains(nativeHost(""), "given.children = painted(inner.__html)") {
		t.Fatal("the tag map does not read the HTML a page injects")
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

func TestNativeAnElementOfTheDevelopmentRuntimeIsBuiltByTheRealOne(t *testing.T) {
	// Measured on the app of the case: the language select of the navbar stopped
	// the first screen with "undefined is not a function". The development runtime
	// is a module of its own -- react/jsx-runtime has no jsxDEV -- so wrapping the
	// one that is not there builds an element with a function that does not exist.
	if strings.Contains(nativeJSXRuntime, "runtime.jsxDEV") {
		t.Fatal("the development runtime is read from the production module")
	}
	for _, said := range []string{
		`import * as devRuntime from "react/jsx-dev-runtime";`,
		"export const jsxDEV = wrap(devRuntime.jsxDEV);",
	} {
		if !strings.Contains(nativeJSXRuntime, said) {
			t.Fatalf("the runtime does not carry %q", said)
		}
	}
}

func TestNativeHTMLIsReadIntoTheTagsTheMapPaints(t *testing.T) {
	// A page that hands HTML to a target with no document: the prop is a string of
	// tags and the only reader of it is a browser. Measured on fonnso, the body of
	// every message the assistant writes is markdown compiled to HTML, and the tags
	// it emits -- p, ul, li, table, pre, code, strong, em, del, a, span and the rest
	// -- are every one of them in the map, so the only thing missing was the reading.
	host := nativeHost("")
	for _, wanted := range []string{
		// The tree is read into the components of the table, and a tag with no
		// component keeps its text instead of taking the render down with it.
		`const component = element.tag === "p" ? Box : hostComponent(element.tag);`,
		"return createElement(Fragment, { key }, ...(children as []))",
		// The classes of the HTML are the classes of the app: a browser writes
		// `class` and nativewind reads `className`.
		`if (name === "class") {
      out.className = value;`,
		// The parser is asked for only by a page that injects HTML.
		`const { parseHTML } = require("./html")`,
	} {
		if !strings.Contains(host, wanted) {
			t.Fatalf("the tag map does not carry %q", wanted)
		}
	}
	for _, tag := range []string{"p", "ul", "li", "table", "pre", "code", "strong", "em", "del", "a", "span", "blockquote", "h2"} {
		if !nativeTagIsNamed(tag) && !strings.Contains(nativeHost(""), `"`+tag+`"`) {
			t.Fatalf("<%s> is in the HTML the app injects and the map does not paint it", tag)
		}
	}
}

func TestNativeHTMLParserIsWrittenAndShipped(t *testing.T) {
	// The parser is the risky half of painting injected HTML, and it is generated
	// rather than written by the app, so it has to be readable here: the tree, the
	// entities, the elements that close themselves, and the quotes a `>` can hide in.
	for _, wanted := range []string{
		"export function parseHTML(html: string): Node[]",
		"const VOID = [",
		"function decoded(text: string): string",
		"function tagEnd(html: string, from: number): number",
		"function attributes(source: string): Record<string, string>",
	} {
		if !strings.Contains(nativeHTML, wanted) {
			t.Fatalf("the parser does not carry %q", wanted)
		}
	}
	shipped := false
	for _, file := range nativeProjectFiles {
		if file.Name == "bifrost/html.ts" && file.Content == nativeHTML {
			shipped = true
		}
	}
	if !shipped {
		t.Fatal("the parser is not part of the generated project")
	}
	// A backslash that survives the Go raw string twice is a regex that matches a
	// backslash: the parser would read a name where the HTML has none, and nothing
	// would say so.
	if strings.Contains(nativeHTML, `\\s`) {
		t.Fatal("a regex of the parser carries an escaped backslash")
	}
}

func TestNativeTakesTheTagOfASelectorDownToAClass(t *testing.T) {
	// A browser answers `.md p` and a native view has no tag to name, so the
	// compiler drops the rule in silence. What a native view has is a class, and
	// the map paints every element with one named after its tag -- so the
	// stylesheet keeps the app's own words and the rewrite is what pays.
	// Measured with the real compiler over the app of the case: 12 rules of its
	// markdown reached the collection, and 29 do with this.
	shipped := false
	for _, file := range nativeProjectFiles {
		if file.Name == "bifrost/selectors.js" && file.Content == nativeSelectors {
			shipped = true
		}
	}
	if !shipped {
		t.Fatal("the rewrite is not part of the generated project")
	}
	if !strings.Contains(nativeSelectors, `tag.replaceWith(parser().className({ value: classOf(tag.value) }))`) {
		t.Fatal("a tag is not written as a class")
	}
	if !strings.Contains(nativeSelectors, `return "native-" + tag;`) {
		t.Fatal("the class is not named after the tag")
	}
	// :where(a, b) is two selectors that mean the same rule, and a native target
	// has nothing to hold them together.
	if !strings.Contains(nativeSelectors, "function written(selector)") ||
		!strings.Contains(nativeSelectors, "firstWhere(copy).replaceWith(...alternative.nodes.map((one) => one.clone()));") {
		t.Fatal(":where() is not written out into the selectors it stands for")
	}
	// The two things a native view has no counterpart for, and which the
	// compiler drops in the same silence.
	if !strings.Contains(nativeSelectors, "pseudo.value.startsWith(\"::\") || STRUCTURAL.test(pseudo.value)") {
		t.Fatal("a pseudo-element and a position in the tree are not reported")
	}
	// The steps of an animation are rules whose selector is not one.
	if !strings.Contains(nativeSelectors, `parent.name.indexOf("keyframes") >= 0`) {
		t.Fatal("the steps of an animation would be renamed")
	}
	// The map paints the class, before the element's own, which is the order a
	// browser settles two rules in.
	if !strings.Contains(nativeHostBody, "function tagged(out: Record<string, unknown>, tag?: string)") {
		t.Fatal("the map does not paint the class of the tag")
	}
	if !strings.Contains(nativeHostBody, `out.className = typeof own === "string" && own !== "" ? tagClass(tag) + " " + own : tagClass(tag);`) {
		t.Fatal("the class of the tag does not come before the element's own classes")
	}
	if !strings.Contains(nativeHostBody, "...htmlProps(element.props, element.tag)") {
		t.Fatal("the HTML a page injects is not painted with the class of its tag")
	}
	// The tag is passed on only when the element is painted by a component of the
	// table: a name it does not know is left as it was, and asking twice would
	// say twice that there is no counterpart.
	if !strings.Contains(nativeJSXRuntime, "typeof painted === \"string\" ? undefined : type") {
		t.Fatal("a name the table does not know would be painted as a class")
	}
	if !strings.Contains(nativeReact, "component === undefined ? undefined : type") {
		t.Fatal("the road through React.createElement does not pass the tag")
	}
	if !strings.Contains(nativeCSSWarnings, `withTagClasses(withoutAnimations(expanded), lostSelectors(config, projectRoot, filePath, options))`) {
		t.Fatal("the transformer does not run the rewrite on what the pipeline produced")
	}
	if !strings.Contains(nativeCSSWarnings, `options.platform === "web" || !isStylesheet(filePath)`) {
		t.Fatal("the rewrite runs on the web target too")
	}
}

func TestNativeTakesTheTagOfASelectorDownToAClassInAStylesheet(t *testing.T) {
	// The module is a string of Go, so nothing else executes it. This runs it
	// against the real compiler: a rule that names a tag has to come out as a
	// class, and the compiler has to keep it.
	node, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun is not installed")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, node, "./scripts/native-selectors.mjs")
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("the rewrite failed: %v: %s", err, output)
	}
	if !strings.Contains(string(output), "todos bien") {
		t.Fatalf("the rewrite is not what it says: %s", output)
	}
}

func TestNativeMakesABlockThatScrollsAScrollView(t *testing.T) {
	// A native view only scrolls when it is a ScrollView, and the classes that
	// ask for a scroll are not declarations the compiler can express: measured on
	// the app of the case, 36 uses in 16 files, and every panel of the chat came
	// out with its content clipped and no way to reach the rest.
	shipped := false
	for _, file := range nativeProjectFiles {
		if file.Name == "bifrost/scroll.js" && file.Content == nativeScroll {
			shipped = true
		}
	}
	if !shipped {
		t.Fatal("the decision of what scrolls is not part of the generated project")
	}
	if !strings.Contains(nativeHostHead, `import { isBottomAnchored, isFixed, isSticky, scrollOf, splitScroll } from "./scroll";`) {
		t.Fatal("the map does not read the decision")
	}
	if !strings.Contains(nativeHostBody, "function Scroller({ className, contentContainerStyle, pinnedInScroller, ref, children, ...rest }") {
		t.Fatal("the map does not paint a block that scrolls")
	}
	if !strings.Contains(nativeHostBody, "<ScrollView") {
		t.Fatal("a block that scrolls is not a ScrollView")
	}
	// The box that is measured stays the box it was: with the app's own onLayout
	// on the ScrollView the whole screen went into a remount cycle -- measured,
	// ~25 warnings a second from react-native-css against 5 in 30 s without it --
	// because the height a ScrollView reports is not the height the parent
	// measured from, so the measurement feeds the layout back.
	if !strings.Contains(nativeHostBody, "<View ref={node} className={split.className} {...rest}>") {
		t.Fatal("the block that scrolls is not wrapped by the box it was")
	}
	if !strings.Contains(nativeHostBody, "style={SCROLL_FILLS}") {
		t.Fatal("the scroll view inside the block does not fill it")
	}
	// With the keyboard up a scroll view eats the first tap: measured, the send
	// button of a form focused with the keyboard open had to be pressed twice.
	if !strings.Contains(nativeHostBody, `keyboardShouldPersistTaps="handled"`) {
		t.Fatal("a tap with the keyboard up would be eaten")
	}
	// React Native refuses a style with alignItems or justifyContent on the scroll
	// view itself and does not render at all: measured, the whole app came out a
	// "ScrollView child layout" render error. They belong to the content, which is
	// what they mean in a browser too.
	if !strings.Contains(nativeHostBody, "const split = splitScroll(className);") {
		t.Fatal("the layout of the children is left on the box that clips")
	}
	if !strings.Contains(nativeScroll, "function splitScroll(className)") {
		t.Fatal("nothing takes the layout of the children out of the class")
	}
	if !strings.Contains(nativeHostBody, "contentContainerStyle={content as never}") {
		t.Fatal("the layout of the children does not reach the content")
	}
	// The direction a ScrollView takes is one, and the class that asks for both
	// is answered with the one that reaches what is below the fold.
	if !strings.Contains(nativeScroll, `if (down && across) {`) {
		t.Fatal("a block that asks for both directions is not answered")
	}
	if !strings.Contains(nativeScroll, `return "vertical";`) {
		t.Fatal("the vertical scroll is not the one a block that asks for both gets")
	}
	// A class the map paints is not a class the target dropped: saying it was
	// would be a line in the log about something that works.
	if !strings.Contains(nativeCSSWarnings, `const { scrollOf } = require("./scroll");`) {
		t.Fatal("the report does not know what the map paints")
	}
	if !strings.Contains(nativeCSSWarnings, "const name = at.charAt(0) === \".\" ? at.slice(1) : at;") || !strings.Contains(nativeCSSWarnings, "if (scrollOf(name) !== null) {") {
		t.Fatal("the report would call a class the map paints a dropped one")
	}
}

func TestNativeAFieldKeepsTheTextItIsGiven(t *testing.T) {
	// Measured on the login of the app of the case: the address typed into the
	// field of the web was clipped in half, because a native input keeps the
	// padding of its font around a line the page sized.
	for _, said := range []string{"includeFontPadding: false", "textAlignVertical: \"center\" as const", "INPUT_FIT"} {
		if !strings.Contains(nativeTextModule, said) {
			t.Fatalf("a field of the page clips what is typed: %q", said)
		}
	}
}

func TestNativeADrawingOfThePageIsFetchedAndPainted(t *testing.T) {
	// The logo of the app of the case is an svg, and the image of a target is not
	// a drawing: the navbar painted nothing where the brand is.
	for _, said := range []string{"SvgUri", `/\.svg(\?|#|$)/i.test(uri)`, "function imageUri(src: unknown)"} {
		if !strings.Contains(nativeHostBody, said) {
			t.Fatalf("a drawing of the page paints nothing: %q", said)
		}
	}
}

func TestNativeAParagraphOfTheHTMLIsABlock(t *testing.T) {
	// Measured on the body of a post of the app of the case: every paragraph came
	// out as one run of text. A p a page writes by hand flows inline with its
	// links; a p of the HTML is the block a browser paints, with its own margins.
	if !strings.Contains(nativeHostBody, `const component = element.tag === "p" ? Box : hostComponent(element.tag);`) {
		t.Fatal("a paragraph of the HTML is painted inline")
	}
	if !strings.Contains(nativeHostBody, `"mark", "p", "pre"`) {
		t.Fatal("a paragraph a page writes by hand lost its inline flow")
	}
}

func TestNativeKeepsTheCookieTheServerSets(t *testing.T) {
	// Measured on the login of the app of the case: the session POST answered with
	// its cookie and the page that followed was fetched as an anonymous visitor.
	for _, said := range []string{
		"const jar = new Map<string, string>();",
		"function readCookies(response: Response): void",
		`jar.set(name, value);`,
		`headers.set("cookie", cookie);`,
		`return original(target, sent).then((response) => {`,
	} {
		if !strings.Contains(nativeNet, said) {
			t.Fatalf("the target forgets the cookie of the server: %q", said)
		}
	}
}

func TestNativeLeavesOutAPlaceThatIsNotANumber(t *testing.T) {
	// A page that places a popup works out where it goes by measuring the page,
	// and what it measured travels as a style of a browser: a box fixed to the
	// viewport and a transform of translate(544px, 130px). Measured on the menu of
	// the language of the case: the box arrived fixed and the transform as a
	// string, which react-native refuses -- "Transform with key translate must be
	// an string with 1 or 2 parameters" -- and a place whose numbers are not
	// numbers has no answer at all.
	for _, said := range []string{
		"export function placed(style: unknown): unknown {",
		`if (given.position === "fixed")`,
		"given.transform = [{ translateX: Number(translate[1]) }, { translateY: Number(translate[2]) }];",
		"const transform = given.transform;",
		"return style.map(placed);",
	} {
		if !strings.Contains(nativePlace, said) {
			t.Fatalf("the place of the page is not read: %q", said)
		}
	}
	// A keyframes of the web needs an interpolator the runtime may not carry: the
	// target paints without animation rather than stopping at the first press.
	for _, said := range []string{"function withoutAnimations(css)", "withTagClasses(withoutAnimations(expanded)"} {
		if !strings.Contains(nativeCSSWarnings, said) {
			t.Fatalf("an animation of the web reaches the renderer: %q", said)
		}
	}
	if !strings.Contains(nativeHostBody, "given.style = placed(given.style);") {
		t.Fatal("the style of an element does not go through it")
	}
	// A menu of the web opens when the pointer goes down, and a target has only a
	// press: a trigger that listened to onPointerDown never opened.
	if !strings.Contains(nativeHostBody, `given.onPress === undefined && typeof given.onPointerDown === "function"`) {
		t.Fatal("a menu that opens on a pointer never opens")
	}
	// Every box of the target goes through it too: the wrapper of a popup is a
	// div, and its style is the one a browser placed.
	for _, said := range []string{
		`style={placed(style) as StyleProp<ViewStyle>}`,
		`[placed(style), textStyle(own, families)]`,
	} {
		if !strings.Contains(nativeTextModule, said) {
			t.Fatalf("a box of the target does not read the place of its style: %q", said)
		}
	}
}

func TestNativeAFragmentDoesNotCutARunOfText(t *testing.T) {
	// A fragment is nothing to a browser -- its children are the children of the
	// box that holds it -- and to react-native it is not: a string that came
	// inside one stays outside the Text, which is the only element that can hold
	// a string. Measured on the organization panel of the case,
	// `{slug}{" / "}<Button>...</Button>` with the slash inside a fragment:
	// react-native answered "Text strings must be rendered within a <Text>
	// component", the slash was not painted, and the button fell to the next line
	// where the browser has both on the same one.
	if !strings.Contains(nativeRuns, "function flatten(children, isFragment) {") {
		t.Fatal("a fragment would cut the run of text it sits in")
	}
	if !strings.Contains(nativeRuns, "for (const child of flatten(children, isFragment)) {") {
		t.Fatal("the run of text does not walk the children of a fragment")
	}
	if !strings.Contains(nativeHostBody, "child.type === Fragment,") {
		t.Fatal("the map does not say which child is a fragment")
	}
}

func TestNativeASubmitButtonAnswersTheEnterOfAField(t *testing.T) {
	// A page writes onKeyDown for the key that ends the typing, and it never
	// fires here: measured on the app of the case, the composer of the chat
	// answered Enter and the device did nothing, so the button was the only way
	// to send. The event a page reads carries the key and the form it is inside.
	if !strings.Contains(nativeHostBody, "onSubmitEditing={onKeyDown === undefined ? undefined : enter}") {
		t.Fatal("the Enter of a field does not answer the page")
	}
	if !strings.Contains(nativeHostBody, `key: "Enter",`) {
		t.Fatal("the event of a key says nothing about the key")
	}
	if !strings.Contains(nativeHostBody, "currentTarget: { form: { requestSubmit: () => form?.submit() } }") {
		t.Fatal("a page that submits the form from the event has nothing to submit")
	}
	// A field of many rows wraps and does not turn into a column of one line per
	// word; one of one row keeps the behaviour of an input.
	if !strings.Contains(nativeHostBody, `multiline={typeof rows === "number"}`) {
		t.Fatal("a textarea is not painted as a field of many rows")
	}
	if !strings.Contains(nativeHostBody, `submitBehavior={typeof rows === "number" ? "submit" : "blurAndSubmit"}`) {
		t.Fatal("the key that ends the typing is not the one the browser uses")
	}
}

func TestNativeWatchesAStreamOfItsOwnServer(t *testing.T) {
	// A page watches its server through `new EventSource(url)`, and there is no
	// such thing on a native target: measured, the live path of the app of the
	// case died with `ReferenceError: Property 'EventSource' doesn't exist`, and
	// the answer did not appear while it was being written.
	shipped := false
	for _, file := range nativeProjectFiles {
		if file.Name == "bifrost/events.ts" && file.Content == nativeEventSource {
			shipped = true
		}
	}
	if !shipped {
		t.Fatal("the stream is not part of the generated project")
	}
	// It is built on XMLHttpRequest and not on fetch, and that is a measurement:
	// a streaming read through the fetch of this target never hands back a body,
	// and the same stream through XMLHttpRequest arrives in parts.
	if !strings.Contains(nativeEventSource, "new globalThis.XMLHttpRequest()") {
		t.Fatal("the stream is not read through XMLHttpRequest")
	}
	if !strings.Contains(nativeDOM, `require("./events").EventSource`) {
		t.Fatal("the shim does not install the stream")
	}
	if !strings.Contains(nativeDOM, "if (target.EventSource === undefined) {") {
		t.Fatal("the shim would replace a stream the target already has")
	}
	// A connection says its whole text every time it changes, so what is new is
	// what was not read yet.
	if !strings.Contains(nativeEventSource, "request.responseText.slice(this.read)") {
		t.Fatal("the whole text of the connection would be read again on every change")
	}
	// Measured: a server with nothing to say yet sends no text, so the headers
	// are what says the connection is open.
	if !strings.Contains(nativeEventSource, "if (request.readyState >= 2 && this.readyState === CONNECTING) {") {
		t.Fatal("the stream is only open once it has text")
	}
	if !strings.Contains(nativeEventSource, "if (request.readyState === 3 && request.responseText.length > this.read) {") {
		t.Fatal("a connection that says the same text again would be read as new")
	}
	// A browser asks again when a stream drops, and it says where it left off, or
	// the whole log arrives once more.
	if !strings.Contains(nativeEventSource, "this.again()") ||
		!strings.Contains(nativeEventSource, `request.setRequestHeader("Last-Event-ID", this.lastEventId)`) {
		t.Fatal("a connection that drops is not continued where it left off")
	}
	// A server that refuses -- the thread is not there, the request is not
	// allowed -- refuses again: asking forever is asking the same thing forever.
	if !strings.Contains(nativeEventSource, "request.status >= 400") {
		t.Fatal("a refused stream would be asked again forever")
	}
	// The reader of the protocol is in the module, where the harness can run it.
	if !strings.Contains(nativeEventSource, "export class Messages {") {
		t.Fatal("the reader of the protocol is not where it can be measured")
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
	// A file that is not a stylesheet is handed on with the same data and the
	// same options, and so is the web target.
	if !strings.Contains(nativeCSSWarnings, "if (options.type === \"asset\" || options.platform === \"web\" || !isStylesheet(filePath)) {") {
		t.Fatal("a file that is not a stylesheet is not handed on unchanged")
	}
	// The stylesheet is compiled here and in this order: the web pipeline
	// expands it, the selectors that name a tag become classes, and the result is
	// compiled. The runtime the app installed would compile the file as it is,
	// before Tailwind has expanded a directive.
	if !strings.Contains(nativeCSSWarnings, `const web = await worker.transform(config, projectRoot, filePath, data, { ...options, platform: "web" });`) {
		t.Fatal("the stylesheet is not expanded by the web pipeline first")
	}
	if !strings.Contains(nativeCSSWarnings, "const compiled = compile(source, {") {
		t.Fatal("the expanded and rewritten stylesheet is not the one compiled")
	}
	if !strings.Contains(nativeCSSWarnings, "upstream.transform(config, projectRoot, filePath + \".js\", injected(compiled.stylesheet()), options)") {
		t.Fatal("the compiled module does not reach the transformer the app installed")
	}
	if !strings.Contains(nativeCSSWarnings, `import { StyleCollection } from "react-native-css/native-internal";`) {
		t.Fatal("the module does not inject the compiled sheet into the collection")
	}
	if !strings.Contains(nativeCSSWarnings, "report(projectRoot, filePath, source, compiled.warnings())") {
		t.Fatal("the wrapper does not read the warnings it exists for")
	}
	// The warning is only actionable if it names the class the app wrote, which
	// means pairing each dropped value with the selector that asked for it.
	if !strings.Contains(nativeCSSWarnings, "declaration.value.includes(String(value))") {
		t.Fatal("a dropped value is not traced back to its class")
	}
	// The class reaches the stylesheet escaped -- .h-[100dvh] is written
	// .h-\[100dvh\] -- and the app can only search for the name it wrote.
	if !strings.Contains(nativeCSSWarnings, "const at = className(selector);") {
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
	if !strings.Contains(nativeCSSWarnings, `options.type === "asset"`) {
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

func TestNativeFlexContainersGetTheRowABrowserGivesThem(t *testing.T) {
	// A browser lays a flex container out along the row and a native view lays
	// its children down the page, so Tailwind's `flex` -- a `display` and nothing
	// else -- is a column here. Measured on device: two boxes of `h-4 w-8` under
	// `flex` stacked one above the other, and the same two under `flex-row` went
	// side by side. The direction goes into the rule that asked for the flex, or
	// the later `.flex-col` Tailwind emits would stop winning with it.
	shipped := map[string]bool{}
	for _, file := range nativeProjectFiles {
		if file.Name == "bifrost/row-direction.js" {
			shipped["row"] = file.Content == nativeRowDirection
		}
		if file.Name == "bifrost/native-internal.js" {
			shipped["internal"] = file.Content == nativeNativeInternal
		}
	}
	if !shipped["row"] {
		t.Fatal("the row direction is not part of the generated project")
	}
	if !shipped["internal"] {
		t.Fatal("the module that wraps the collection is not part of the generated project")
	}
	if !strings.Contains(nativeRowDirection, `group.flexDirection = "row";`) {
		t.Fatal("the direction is not written into the rule that asked for the flex")
	}
	if !strings.Contains(nativeRowDirection, `names(declarations, "flexDirection")`) {
		t.Fatal("a rule that already names a direction would be given another")
	}
	// A declaration whose value is a variable is an array in the collection, and
	// reading one of those as an object would take a variable for a display.
	if !strings.Contains(nativeRowDirection, "Array.isArray(group)") {
		t.Fatal("a declaration that arrives through a variable is read as one that does not")
	}
	// The collection is one object for the whole runtime, so the module is
	// patched where it is read and the same module is handed on: a copy would
	// leave every other reader with the copy's opposite.
	if !strings.Contains(nativeNativeInternal, "const inject = collection.inject.bind(collection);") {
		t.Fatal("the collection is not read as it is handed in")
	}
	if !strings.Contains(nativeNativeInternal, "module.exports = upstream;") {
		t.Fatal("the wrapper does not hand the module over")
	}
	if !strings.Contains(nativeMetroHelper, `moduleName === "react-native-css/native-internal"`) {
		t.Fatal("the generated project does not resolve the collection through the wrapper")
	}
	if !strings.Contains(nativeMetroHelper, "if (moduleName === \"react-native-css/native-internal\" && !insideBifrost) {") {
		t.Fatal("the wrapper would wrap itself")
	}
	if !strings.Contains(nativeMetroHelper, `path.join(bifrost, "native-internal.js")`) {
		t.Fatal("the resolver does not point at the wrapper")
	}
}

func TestNativeWarningsCompileWhatThePipelineProduced(t *testing.T) {
	// Metro does not call a transformer with the config the app wrote: it hands it
	// `config.transformer` on its own, flattened, minus the three keys it keeps for
	// itself. Reading the kept path anywhere but the top of that object finds
	// nothing, and the wrapper then falls back to the default transformer, which
	// for a stylesheet on a native target is an empty module. Measured against a
	// real app: the whole stylesheet went out, the app painted unstyled, and
	// nothing in the log said so.
	//
	// What the wrapper does with a stylesheet is the order of the steps: the web
	// pipeline expands Tailwind, and only what comes out is compiled. The runtime
	// the app installed would compile the file as it is, so the preflight -- the
	// rule that puts the typeface of the theme on html, every reset that names a
	// tag -- reached the target as a selector no view answers and was dropped.
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
	if _, err := writeNativeProject(native, prepared, "", filepath.Join(projectRoot, "native")); err != nil {
		t.Fatal(err)
	}

	// The compiler the helper looks for when it decides whether to install the
	// wrapper, and the modules the wrapper requires at the top of itself.
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
	stub("react-native-css", `module.exports = {
  compile: (css) => ({ warnings: () => ({}), stylesheet: () => ({ compiled: css }) }),
};
`)
	if err := os.WriteFile(filepath.Join(native, "node_modules", "react-native-css", "compiler.js"), []byte("module.exports = require(\"./index.js\");\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The transformer Metro would run if this one were not installed: the web
	// pass answers with a stylesheet of its own, and any other pass is the one the
	// app installed answering with what it was given.
	stub("@expo/metro-config", `module.exports = {
  unstable_transformerPath: require.resolve("./index.js"),
  transform: async (config, root, filePath, data, options) => {
    if (options.platform === "web") {
      return { output: [{ data: { css: { code: "html { font-family: serif }" } } }] };
    }
    return { output: [{ data: { code: "UPSTREAM " + filePath + " " + data.toString() } }] };
  },
};
`)

	probe := `import { createRequire } from "node:module";
const require = createRequire(import.meta.url);
const path = require("node:path");
const fs = require("node:fs");
const { withBifrostWarnings } = require("./bifrost/metro.js");
const upstream = path.join(process.cwd(), "upstream.js");
fs.writeFileSync(upstream, "module.exports.transform = async (config, root, filePath, data) => ({ output: [{ data: { code: \"APP \" + filePath + \" \" + data.toString() } }] });\n");
const warned = withBifrostWarnings({ transformerPath: upstream, transformer: { reactNativeCSS: {} } });
const { getTransformOptions, transformVariants, unstable_workerThreads, ...transformerConfig } = warned.transformer;
const wrapper = require(warned.transformerPath);
const options = { platform: "android", dev: false, type: "module", minify: false, customTransformOptions: {} };
const sheet = await wrapper.transform(transformerConfig, process.cwd(), path.join(process.cwd(), "global.css"), Buffer.from("@import \"tailwindcss\";"), options);
const other = await wrapper.transform(transformerConfig, process.cwd(), path.join(process.cwd(), "bifrost", "net.ts"), Buffer.from("export const net = 1;"), options);
console.log(JSON.stringify({ sheet: sheet.output[0].data, other: other.output[0].data }));
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
		Sheet struct {
			Code string `json:"code"`
			CSS  struct {
				Code string `json:"code"`
			} `json:"css"`
		} `json:"sheet"`
		Other struct {
			Code string `json:"code"`
		} `json:"other"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(output))), &got); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	// The stylesheet the app installed got is the compiled one, and what was
	// compiled is what the web pipeline produced.
	if !strings.Contains(got.Sheet.Code, `StyleCollection.inject({"compiled":"html { font-family: serif }"})`) {
		t.Fatalf("the compiled sheet is not the one injected: %s", got.Sheet.Code)
	}
	if !strings.HasSuffix(strings.Fields(got.Sheet.Code)[1], "global.css.js") {
		t.Fatalf("the module was not handed on as a module: %s", got.Sheet.Code)
	}
	if strings.Contains(got.Sheet.Code, `@import \"tailwindcss\"`) {
		t.Fatalf("the stylesheet was compiled before the pipeline expanded it: %s", got.Sheet.Code)
	}
	// Metro asked for the CSS of this file and got a module: what it keeps as an
	// asset is empty.
	if got.Sheet.CSS.Code != "" {
		t.Fatalf("the sheet is also emitted as an asset: %q", got.Sheet.CSS.Code)
	}
	// A file that is not a stylesheet reaches the app's transformer unchanged, with
	// its path and its bytes.
	if !strings.Contains(got.Other.Code, "APP "+filepath.Join(native, "bifrost", "net.ts")+" export const net = 1;") {
		t.Fatalf("a file that is not a stylesheet did not reach the app's transformer: %s", got.Other.Code)
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
	// The two imports of the fonts come after the shim: the layout loads the
	// typeface of the app before it paints, and an app that ships none has an
	// empty map and the first frame.
	fonts := []string{`import * as declaredFonts from "../bifrost/fonts";`, `import { useFonts } from "expo-font";`}
	cases := []struct {
		name    string
		files   []string
		imports []string
	}{
		{
			"an overlay with neither",
			nil,
			append([]string{`import "../bifrost/net";`}, fonts...),
		},
		{
			"a global stylesheet",
			[]string{"global.css"},
			append([]string{`import "../global.css";`, `import "../bifrost/net";`}, fonts...),
		},
		{
			"a module of its own",
			[]string{"bifrost/setup.ts"},
			append([]string{`import "../bifrost/setup";`, `import "../bifrost/net";`}, fonts...),
		},
		{
			"both, before the shim that wraps what they install",
			[]string{"global.css", "bifrost/setup.tsx"},
			append([]string{`import "../global.css";`, `import "../bifrost/setup";`, `import "../bifrost/net";`}, fonts...),
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
	if !strings.Contains(net, `url.startsWith("/")`) || !strings.Contains(net, "original(target, sent)") {
		t.Fatal("the shim does not do what the browser does to a relative url")
	}
	// A multipart request with a file is the one the fetch of the runtime cannot
	// send: the uploader of the target is the one that knows the part.
	for _, wanted := range []string{
		"function isUpload(body: unknown): body is FormData",
		"function uploaded(url: string, init: RequestInit): Promise<Response>",
		"new XMLHttpRequest()",
		"isUpload(init.body)",
	} {
		if !strings.Contains(net, wanted) {
			t.Fatalf("the shim does not send an upload: %q", wanted)
		}
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
		// A control asks what an element is with instanceof, and a name that does
		// not exist is a ReferenceError and not a false.
		`"HTMLFormElement"`,
		// A control writes a native element through the descriptor of its prototype.
		`property("HTMLInputElement", "checked");`,
		// Closing a sheet through its X, the library that hands focus back asks
		// "event instanceof win.KeyboardEvent": an instanceof whose right side is
		// not there is a throw, and in a release build that throw ends the app.
		"target.KeyboardEvent ??=",
		"target.FocusEvent ??=",
		"target.MouseEvent ??=",
		// The back button of the phone closes a dialog through the keydown the
		// dialog listens for on the document, and the document has to answer
		// whether anyone was listening.
		"dispatchEvent: dispatch,",
		"target.navigator.clipboard",
	} {
		if !strings.Contains(dom, said) {
			t.Fatalf("the shim does not answer %s", said)
		}
	}
	// A cookie a page writes is one it reads again: the theme of the app is set
	// through document.cookie, and a getter with no setter stops the effect that
	// sets it.
	if !strings.Contains(dom, "set cookie(value) {") || !strings.Contains(dom, "return cookieText();") || !strings.Contains(dom, "function writeCookie(text) {") {
		t.Fatal("the shim does not answer the cookie a page wrote")
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
	// The back button is handed to whatever listens for Escape, and it stays with
	// the dialog when there is one -- measured on the sidebar, where it took the
	// app away with the sheet open.
	if !strings.Contains(nativeLayout(""), "hardwareBackPress") {
		t.Fatal("the back button of the phone leaves the app with a dialog open")
	}
}

func TestNativeEveryGeneratedJavaScriptIsJavaScript(t *testing.T) {
	// Measured: one type annotation left in a generated .js file stops tsc on
	// the whole generated project before it reads a line of the app, and a
	// project that does not typecheck is a project no editor will help with.
	annotation := regexp.MustCompile(`\)\s*:\s*(string|number|boolean|void|any|unknown)\b|\(\s*[A-Za-z_$][A-Za-z0-9_$]*\s*:\s*(string|number|boolean|void|any|unknown)\b`)
	for _, file := range nativeProjectFiles {
		if !strings.HasSuffix(file.Name, ".js") {
			continue
		}
		if at := annotation.FindStringIndex(file.Content); at != nil {
			t.Fatalf("%s carries a type annotation: %q", file.Name, file.Content[at[0]:at[1]])
		}
	}
}

func TestNativeARefIsAnElementOfTheDocument(t *testing.T) {
	// Measured on the app of the case: the select of a language toggle asks its
	// trigger for the closest form, and a native view has no closest; the content
	// of the same select reaches its window through node.ownerDocument.defaultView.
	if !strings.Contains(nativeHostHead, "function useElementRef(ref: unknown)") {
		t.Fatal("the map hands the native view to a ref instead of an element")
	}
	for _, said := range []string{"ref={node}", "ref={useElementRef(ref)}"} {
		if !strings.Contains(nativeHostBody, said) {
			t.Fatalf("the map does not carry %q", said)
		}
	}
	for _, said := range []string{"insertAdjacentElement: () => null", "remove: () => undefined", "createTreeWalker: () => ({ nextNode: () => null })", "target.NodeFilter ??="} {
		if !strings.Contains(nativeDOM, said) {
			t.Fatalf("a focus guard has no place in the body: %q", said)
		}
	}
	for _, said := range []string{"const viewport = () => globalThis.__bifrostViewport", "clientWidth", "clientHeight"} {
		if !strings.Contains(nativeDOM, said) {
			t.Fatalf("a popup has no viewport to keep itself inside: %q", said)
		}
	}
	if !strings.Contains(nativeDOM, `target.MutationObserver ??= class MutationObserver {`) {
		t.Fatal("a focus scope that observes a box has no observer to ask for")
	}
	if !strings.Contains(nativeDOM, "node.ownerDocument = target.document;") {
		t.Fatal("an element made by the document does not know its document")
	}
	for _, said := range []string{"measureInWindow", "__rect = { x, y, width, height", "__bifrostViewport"} {
		if !strings.Contains(nativeHostHead, said) {
			t.Fatalf("a ref does not carry the box of its view: %q", said)
		}
	}
}

func TestNativeTheMapAsksTheRouterWhenALinkIsPressed(t *testing.T) {
	// Measured on the app of the case: the map of tags is loaded from the JSX
	// runtime while expo-router may still be evaluating, and an import of the
	// router there was a cycle -- expo-router came back half built, the map
	// stopped halfway, and the first screen painted an Image that was undefined.
	if strings.Contains(nativeHostHead, `import { navigate } from "virtual:bifrost/navigation"`) {
		t.Fatal("the map imports the router while the page graph is still loading")
	}
	for _, said := range []string{
		"function navigate(href: string): void {",
		`require("virtual:bifrost/navigation")`,
	} {
		if !strings.Contains(nativeHostHead, said) {
			t.Fatalf("the map does not carry %q", said)
		}
	}
}

func TestNativeTheGeneratedProjectTypechecksAsItIsWritten(t *testing.T) {
	// Measured, with the TypeScript the project installs: the project had a
	// hundred errors, most of them one cause -- a class whose state was only
	// assigned in its constructor is not a class to tsc. What is checked here is
	// what an editor needs to see the project whole.
	files := map[string]string{}
	for _, file := range nativeProjectFiles {
		files[file.Name] = file.Content
	}
	tsconfig := files["tsconfig.json"]
	for _, declared := range []string{
		`"strict": true`,
		`"allowImportingTsExtensions": true`,
		`"virtual:bifrost/navigation": ["./bifrost/navigation"]`,
	} {
		if !strings.Contains(tsconfig, declared) {
			t.Fatalf("the project does not declare %s", declared)
		}
	}
	if !strings.Contains(files["bifrost/types.d.ts"], `declare module "*.css"`) {
		t.Fatal("the layout imports a stylesheet nothing declares")
	}
	for _, field := range []string{
		"rest: string;",
		"data: string[];",
		"id: string | null;",
		"event: string;",
		"request: XMLHttpRequest | null;",
		"handlers: Map<string, Handler[]>;",
	} {
		if !strings.Contains(files["bifrost/events.ts"], field) {
			t.Fatalf("the stream does not declare %q", field)
		}
	}
}

func TestNativeTheBoxAPagePinsKeepsItsPlace(t *testing.T) {
	// Measured on the device: position: fixed compiled to nothing and the header
	// of the shell was painted nowhere; position: sticky was a value the compiler
	// dropped and the box scrolled away with the content below it.
	files := map[string]string{}
	for _, file := range nativeProjectFiles {
		files[file.Name] = file.Content
	}
	position := files["bifrost/position.js"]
	if position == "" {
		t.Fatal("a fixed box has no rewrite of its position")
	}
	for _, said := range []string{
		`root.walkDecls("position"`,
		`declaration.value.trim() === "fixed"`,
		`declaration.value = "absolute"`,
	} {
		if !strings.Contains(position, said) {
			t.Fatalf("the rewrite does not carry %q", said)
		}
	}
	transform := files["bifrost/css-warnings.js"]
	if !strings.Contains(transform, "withFixed(withTagClasses(") {
		t.Fatal("the stylesheet is compiled before the fixed position is rewritten")
	}
	if !strings.Contains(transform, `name === "sticky"`) {
		t.Fatal("a sticky box that the scroll view pins is reported as dropped")
	}
	scroll := files["bifrost/scroll.js"]
	if !strings.Contains(scroll, "function isSticky(") {
		t.Fatal("nothing names the class of a child a scroll view can pin")
	}
	host := nativeHost("")
	if !strings.Contains(host, "stickyHeaderIndices=") || !strings.Contains(host, "isSticky(") {
		t.Fatal("a sticky child of a box that scrolls is not pinned")
	}
}

func TestNativeAPinnedBoxFollowsTheDocument(t *testing.T) {
	// Measured on the device: a fixed box over a page that scrolls went up with
	// the content, and a fixed box of the bottom stayed at the bottom of the
	// content, which is a screen or three below where it belongs. The offset of
	// the document is what the box has to cancel.
	files := map[string]string{}
	for _, file := range nativeProjectFiles {
		files[file.Name] = file.Content
	}
	document := files["bifrost/document-scroll.ts"]
	if document == "" {
		t.Fatal("a pinned box has no document offset to read")
	}
	for _, said := range []string{
		"export const DocumentScroll = createContext<DocumentScrollValue | null>(null);",
		"y: Animated.Value;",
		"viewport: number;",
		"content: number;",
	} {
		if !strings.Contains(document, said) {
			t.Fatalf("the document offset does not carry %q", said)
		}
	}
	for _, said := range []string{
		"<DocumentScroll.Provider value={{ y: scrollY, viewport: viewportHeight, content: contentHeight, inner: innerView }}>",
		"<Animated.ScrollView",
		"onContentSizeChange={(width: number, height: number) => { setContentHeight(height); }}",
		"onLayout={(event) => { setViewportHeight(event.nativeEvent.layout.height); }}",
		"onScroll={Animated.event([{ nativeEvent: { contentOffset: { y: scrollY } } }], { useNativeDriver: true })}",
	} {
		if !strings.Contains(nativeScreen, said) {
			t.Fatalf("the screen does not carry %q", said)
		}
	}
	if host := nativeHost(""); !strings.Contains(host, "bottomAnchored={isBottomAnchored(className)}") {
		t.Fatal("a pinned box does not say whether it is anchored to the bottom")
	}
	text := files["bifrost/text.tsx"]
	for _, said := range []string{
		"const anchored = document !== null && document.viewport > 0 && bottomAnchored === true;",
		"const correction = document === null || !anchored ? 0 : document.viewport - document.content;",
		"Animated.add(document.y, correction)",
	} {
		if !strings.Contains(text, said) {
			t.Fatalf("the pinned box does not carry %q", said)
		}
	}
}

func TestNativeAStickyBoxOfTheDocumentKeepsItsEdge(t *testing.T) {
	// Measured on the device: a sticky title of a page without a scroll box of its
	// own scrolled away with the content. The box is in the flow until its top
	// reaches the edge, and from there it travels with the content by as much as
	// it scrolls; the distance to that edge is measured against the box the
	// document scrolls, and measured again whenever the content changes size.
	files := map[string]string{}
	for _, file := range nativeProjectFiles {
		files[file.Name] = file.Content
	}
	document := files["bifrost/document-scroll.ts"]
	if !strings.Contains(document, "inner: RefObject<View | null> | null;") {
		t.Fatal("the document does not carry the box a sticky one is measured against")
	}
	for _, said := range []string{
		"innerViewRef={innerView as never}",
		"value={{ y: scrollY, viewport: viewportHeight, content: contentHeight, inner: innerView }}",
	} {
		if !strings.Contains(nativeScreen, said) {
			t.Fatalf("the screen does not carry %q", said)
		}
	}
	text := files["bifrost/text.tsx"]
	for _, said := range []string{
		"function StickyViewBox(",
		"node.measureLayout(inner.current",
		"inputRange: [-1, 0, top, top + 1, 1e9], outputRange: [0, 0, 0, 1, 1e9]",
		"export const StyledStickyView = styled(StickyViewBox);",
	} {
		if !strings.Contains(text, said) {
			t.Fatalf("the sticky box does not carry %q", said)
		}
	}
	host := nativeHost("")
	for _, said := range []string{
		`<StickyView ref={node} className={className as string} {...rest}>`,
		"pinnedInScroller !== true",
		"cloneElement(child as never, { pinnedInScroller: true } as never)",
	} {
		if !strings.Contains(host, said) {
			t.Fatalf("the map does not carry %q", said)
		}
	}
}

func TestNativeThePageScrollsAsItsDocument(t *testing.T) {
	// Measured on the device: a page taller than the screen painted its first rows
	// and nothing else, with no way to reach the rest, because a native view does
	// not scroll unless it is a ScrollView. On the web the document is what
	// scrolls, and here the screen is the document.
	for _, said := range []string{
		`import { ActivityIndicator, Animated, StyleSheet, Text, type View as NativeView } from "react-native";`,
		"<Animated.ScrollView",
		"contentContainerStyle={styles.page}",
		`keyboardShouldPersistTaps="handled"`,
		"style={styles.pageScroll}",
		"screen: { flex: 1 }",
		"pageScroll: { flex: 1 }",
		"page: { flexGrow: 1 }",
	} {
		if !strings.Contains(nativeScreen, said) {
			t.Fatalf("the screen does not carry %q", said)
		}
	}
	if !strings.Contains(nativeScreen, "<View className={root} style={documentStyle}>") {
		t.Fatal("the document root does not hold the box that scrolls")
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
	if _, err := writeNativeProject(native, prepared, "", filepath.Join(projectRoot, "native")); err != nil {
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
	if _, err := writeNativeProject(native, prepared, "http://example.test", filepath.Join(projectRoot, "native")); err != nil {
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
	// leaves the document and lands wherever it redirects, with the cookies of the
	// answer. The request is the app's here, so that what the server sets belongs
	// to the client that will use it, and a redirect that leaves this server is
	// handed to the browser, which is where it went before.
	if !strings.Contains(dom, "follow(target).then((answer) => {") {
		t.Fatal("a url that is not a page of the app is never asked for")
	}
	if !strings.Contains(dom, "openElsewhere(answer.href);") {
		t.Fatal("a redirect that leaves this server is not handed to the browser")
	}
	if strings.Index(dom, "openElsewhere(answer.href)") < strings.Index(dom, "follow(target)") {
		t.Fatal("the browser is handed a url before it was ever asked for")
	}
	// And a url of this app's own server that the server answers with no move is
	// never handed over: this app is what the system opens that domain with, so
	// asking for it opens this app again with the same url. Measured on a link of
	// a mail that had already been used: 1158 launches of the app in one minute
	// and a phone that had to be restarted.
	if !strings.Contains(dom, "if (url.indexOf(ORIGIN) === 0) {") {
		t.Fatal("a document of this server would be handed to the browser, which opens this app")
	}
	if !strings.Contains(dom, "is a document of this app's own server and no screen of this app can paint it") {
		t.Fatal("the loop is not said out loud where it is broken")
	}
	if !strings.Contains(dom, "was handed to the browser a moment ago; not again") {
		t.Fatal("the same url can be handed to the browser twice in a row")
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

func TestNativeAsksItsOwnServerForAUrlThatIsNotAPage(t *testing.T) {
	// A page writes the url the browser would leave the document for -- the link
	// that opens a session, the endpoint that starts a checkout -- and on a native
	// target the client that has to make that request is the app: a cookie set into
	// a browser is a session the app will never have. What the server redirects
	// inside its own origin is an address the router can paint, and everything else
	// is the browser's, which is where it went before.
	files := map[string]string{}
	for _, file := range nativeProjectFiles {
		files[file.Name] = file.Content
	}
	intent, ok := files["bifrost/intent.ts"]
	if !ok {
		t.Fatal("a url of the server has nothing to ask for it")
	}
	for _, said := range []string{
		"export async function follow(address: string): Promise<Answered | null>",
		"export function pathOf(url: string): string",
		// The redirect is read and not followed, because the address it names is the
		// whole answer: it says whether the app stays or the browser goes.
		`fetch(ORIGIN + path, { redirect: "manual" })`,
		`response.headers.get("location")`,
		"if (response.status < 300 || response.status > 399) {",
	} {
		if !strings.Contains(intent, said) {
			t.Fatalf("the request of a url of the server does not %s", said)
		}
	}
	// Every url of the server is asked for, the api included. It used to be left
	// to the browser for being the url that creates a checkout session with the
	// request itself, and what that cost is the button of Google: the dance of an
	// oauth starts by asking that url, and the cookie of the state belongs to
	// whoever asked -- so a browser that asks is a browser that ends the dance with
	// a session this app will never have. What is not asked for is a url that the
	// app cannot answer and the server does not move: see the shim.
	if !strings.Contains(intent, "export async function follow(address: string): Promise<Answered | null> {") {
		t.Fatal("a url of the server is asked for somewhere else")
	}
	if !strings.Contains(intent, `return ours === null ? { kind: "outside", href: at } : { kind: "inside", href: ours };`) {
		t.Fatal("a redirect that leaves this server is not told apart from one of ours")
	}
	// An address that looks relative and leaves the origin -- `//elsewhere/x` -- is
	// not one of ours, and the check has to come before the one for a path.
	if !strings.Contains(intent, "address.charAt(1) === \"/\" ? null : address") {
		t.Fatal("a protocol-relative url is taken as a path of the app")
	}
}

func TestNativeTurnsAUrlThatArrivedIntoAMoveOfTheRouter(t *testing.T) {
	// A deep link that opens the app at a url the router has no screen for is what
	// this is for: measured on the app of the case, the link of a mail left the
	// screen on "Unmatched Route" and the session cookie landed nowhere. The file
	// is written by the generator and not by an app, because what it decides is the
	// same for every app.
	projectRoot := t.TempDir()
	if _, err := writeNativeProject(filepath.Join(projectRoot, ".bifrost", "native"), appRouter{ProjectRoot: projectRoot}, "http://127.0.0.1:8080", filepath.Join(projectRoot, "native")); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(projectRoot, ".bifrost", "native", "app", "+native-intent.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	written := string(content)
	for _, said := range []string{
		`import { follow, pathOf } from "../bifrost/intent";`,
		`import { hasPage } from "../bifrost/route";`,
		"export async function redirectSystemPath(",
		"if (hasPage(address)) {",
	} {
		if !strings.Contains(written, said) {
			t.Fatalf("the intent of the system does not %s", said)
		}
	}
	// A url of this app's own server that the server does not answer with a
	// redirect is a document, and this app cannot hand it over: the App Link of
	// this domain is its own, so asking the system to open it opens this app again
	// with the same url. Measured on a link of a mail that had already been used:
	// 1158 launches of the app in one minute and a phone that had to be restarted.
	if strings.Contains(written, "Linking.openURL(ORIGIN +") {
		t.Fatal("a document of this server is handed to the browser, which opens this app again")
	}
	if !strings.Contains(written, "is a document of this app's own server and no screen of this app can paint it") {
		t.Fatal("the loop is not said out loud where it is broken")
	}
	// And a redirect that leaves this server -- the dance of an oauth provider --
	// is the one url the browser has to open, because the browser is where that
	// happens.
	if !strings.Contains(written, "await Linking.openURL(answer.href)") {
		t.Fatal("a redirect that leaves this server does not reach the browser")
	}
	// A path that is a page stays the router's, exactly as it was before this file
	// existed: the url that arrived is the one it is given.
	if !strings.Contains(written, "return path;") {
		t.Fatal("a deep link to a page of the app is not left to the router")
	}
	if !strings.Contains(written, "return path;") || !strings.Contains(written, "return null;") {
		t.Fatal("a url that arrived goes somewhere it was not told to")
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
		// The browser's name for the press is answered, not dropped. The tag a
		// page wrote the element as comes with it, because it is the class the
		// stylesheet of the app reaches the element through.
		"export function nativeProps(props: unknown, tag?: string): unknown",
		// The form keeps its fields and answers the submit with them, in a module
		// of its own, so that a button the app paints itself still submits.
		"const [fields] = useState(() => new Map<string, string>());",
		"body.append(name, value)",
		// A select of more than one value answers with a list for its name.
		"const [lists] = useState(() => new Map<string, string[]>());",
		"for (const [name, values] of lists) {",
		// A file answers with the upload itself.
		"const [files] = useState(() => new Map<string, Upload[]>());",
		"for (const [name, uploads] of files) {",
		"body.append(name, upload as unknown as Blob);",
		// What the page reads out of the event is the FormData the form built.
		"source instanceof NativeFormData",
		// A replacement that did not share the prototype made fetch refuse every
		// upload: "Unsupported BodyInit type".
		"(PatchedFormData as unknown as { prototype: unknown }).prototype = NativeFormData.prototype;",
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
	if !strings.Contains(host, "function Field({ name, value, defaultValue, rows, type, inputMode, onKeyDown, onChange, ...rest }") {
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

func TestNativeTagMapAnswersTheChangeOfAField(t *testing.T) {
	// A page that keeps the value in its own state asks for it with onChange.
	// A field that only tells the form leaves the page's state empty, and the
	// page paints the value it had: measured on the files panel of the app, the
	// search box typed and the list never filtered.
	host := nativeHost("")
	if !strings.Contains(host, "function Field({ name, value, defaultValue, rows, type, inputMode, onKeyDown, onChange, ...rest }") {
		t.Fatal("a field does not listen to its change")
	}
	if !strings.Contains(host, "target: { value: text, name }") {
		t.Fatal("the change of a field does not reach the page")
	}
}

func TestNativeFieldReadsTheTypeABrowserReads(t *testing.T) {
	// A field of type="password" has to mask what is typed, and an email, a
	// number, a phone and a url each bring the keyboard of what they ask for.
	// Measured on fonnso: the password painted in clear text, and the email field
	// opened the plain keyboard with its first letter capitalized.
	host := nativeHost("")
	for _, wanted := range []string{
		"out.secureTextEntry = true",
		"out.keyboardType = \"email-address\"",
		"out.keyboardType = \"decimal-pad\"",
		"out.keyboardType = \"number-pad\"",
		"out.keyboardType = \"phone-pad\"",
		"out.keyboardType = \"url\"",
		"out.autoCapitalize = \"none\"",
		"out.autoCorrect = false",
		"{...fieldControl(type, inputMode)}",
	} {
		if !strings.Contains(host, wanted) {
			t.Fatalf("the field does not answer %q", wanted)
		}
	}
	// inputMode is the keyboard said on its own, and it wins over type for the
	// keyboard while type still wins for the masking.
	if !strings.Contains(host, "const said = inputMode !== undefined && inputMode !== null ? inputMode : type;") {
		t.Fatal("inputMode does not win the keyboard")
	}
}

func TestNativeInlineElementThatAsksForFlexIsABox(t *testing.T) {
	// A label and a span flow as text, but a browser turns one into a box the
	// moment its class asks for a flex layout, and a Text cannot lay children out
	// along an axis. Measured on the login form of the case: every <label> that
	// held a field came out on one line with the field beside the label.
	host := nativeHost("")
	for _, wanted := range []string{
		"function Inline({ className, children, ...rest }",
		"if (asksForFlex(className))",
		"return <Box className={className as string} {...rest}>{children}</Box>;",
		"return <Text className={className as string} {...rest}>{children}</Text>;",
		// The elements of a run of text are the inline ones, and the map hands
		// the rule the same component it paints them with.
		"child.type === Inline",
	} {
		if !strings.Contains(host, wanted) {
			t.Fatalf("the inline element does not answer %q", wanted)
		}
	}
	// The tags that flow as text go through Inline, so a label that asks for flex
	// is a box and a span that does not is still a Text.
	if !strings.Contains(host, "for (const name of TEXTS) {\n  NAMED[name] = Inline;\n}") {
		t.Fatal("the text tags do not go through Inline")
	}
}

func TestNativeSelectIsAListTheTargetCanPaint(t *testing.T) {
	// A select is a list of options in a browser, and the tag map had neither:
	// measured on the device, one <select> in a form took the whole screen down
	// with "View config getter callback for component `option` must be a
	// function". The list is the chosen option in a pressable and the rest in a
	// sheet, built from the elements every target has.
	host := nativeHost("")
	for _, wanted := range []string{
		"select: Select,",
		"option: Option,",
		"optgroup: OptionGroup,",
		"function Select(",
		"function SingleSelect(",
		"function MultipleSelect(",
		"props.multiple === true ? <MultipleSelect {...props} /> : <SingleSelect {...props} />",
		"function choicesOf(",
		"if (child.type === OptionGroup) {",
		"value: typeof props.value === \"string\" ? props.value : label,",
		"const chosen = wanted === undefined ? own : wanted === null ? \"\" : String(wanted);",
		// A page that keeps the value in its own state reads the event a browser
		// gives it.
		"target: { value: choice.value, name },",
		// A field of a form travels with the form even when it is not typed.
		"form.fields.set(name, chosen);",
		// Without a value and without a default, a browser selects the first
		// option that is not disabled.
		"const first = choices.find((choice) => !choice.disabled);",
		"return first === undefined ? \"\" : first.value;",
		// The sheet is what a browser opens, and the back button closes it.
		"onRequestClose={() => setOpen(false)}",
		// A select of more than one value is a list in place, and its name
		// carries every value that is on.
		"form.lists.set(name, chosen);",
		"target: { value: next, selectedOptions, name, type: \"select-multiple\" },",
		"const selectedOptions = next.map((one) => ({ value: one, selected: true }));",
		"accessibilityRole=\"checkbox\"",
	} {
		if !strings.Contains(host, wanted) {
			t.Fatalf("the select does not answer %q", wanted)
		}
	}
	// An option is read by the select that holds it and never painted on its own,
	// wherever it is met.
	for _, name := range []string{"Option", "OptionGroup"} {
		body, err := nativeFunctionBody(host, name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(body, "return null;") {
			t.Fatalf("%s paints something of its own:\n%s", name, body)
		}
	}
}

func TestNativeAControlThatIsNotTextIsItsControl(t *testing.T) {
	// Measured on the device: <input type="checkbox"> came out as an empty text
	// field and a radio as a field holding its own value. A browser draws a box
	// and sends target.checked, and that is what the map paints now.
	host := nativeHost("")
	for _, wanted := range []string{
		"input: InputTag,",
		"return <Field {...props} />;",
		"if (props.type === \"checkbox\") {",
		"if (props.type === \"radio\") {",
		"function CheckBox(",
		"function Radio(",
		"function Control(",
		// A checkbox without a value sends "on", and a radio without one sends
		// nothing, as a browser does.
		"return type === \"checkbox\" ? \"on\" : \"\";",
		// The event carries what a page reads.
		"const target = { checked, value, name, type };",
		// A form keeps the checked box and forgets the unchecked one.
		"form.fields.set(name, sent);",
		"form.fields.delete(name);",
		// A radio group turns the others off by name, and an uncontrolled group
		// keeps the choice itself.
		"const group = checked === undefined && typeof name === \"string\" ? radioGroup(name) : undefined;",
		"group.selected = sent;",
		"for (const listener of group.listeners) {",
		"if (group !== undefined && group.selected === undefined && defaultChecked === true) {",
		// A radio that is already chosen does nothing, as a browser does.
		"if (on) {\n      return;\n    }",
		// The class of the page sizes the box, and a control without one still
		// has a box to be pressed.
		"style={bare ? styles.control : undefined}",
		"accessibilityRole={type === \"radio\" ? \"radio\" : \"checkbox\"}",
	} {
		if !strings.Contains(host, wanted) {
			t.Fatalf("the control does not answer %q", wanted)
		}
	}
}

func TestNativeAnSvgIsADrawingTheTargetCanPaint(t *testing.T) {
	// Measured on the device: an <svg> warned "no native counterpart for <svg>"
	// and painted nothing. react-native-svg is part of the runtime of Expo Go,
	// so the drawing is a drawing here too.
	dependencies, _ := nativePackageManifest()["dependencies"].(map[string]any)
	if dependencies["react-native-svg"] != "15.15.4" {
		t.Fatalf("the project draws an svg without the module that draws it: %v", dependencies["react-native-svg"])
	}
	host := nativeHost("")
	for _, wanted := range []string{
		`import Svg, { Circle, ClipPath, Defs, Ellipse, G, Line, LinearGradient, Mask, Path, Polygon, Polyline, RadialGradient, Rect, Stop, SvgUri, Symbol as SymbolTag, Use } from "react-native-svg";`,
		"svg: SvgRoot,",
		"path: SvgPath,",
		"g: SvgG,",
		"linearGradient: SvgLinearGradient,",
		"clipPath: SvgClipPath,",
		// The class sizes the box and the drawing fills it; a drawing without a
		// class keeps the size the page wrote.
		"const sized = flat.width !== undefined || flat.height !== undefined;",
		"width: sized ? \"100%\" : props.width,",
		"height: sized ? \"100%\" : props.height,",
		// The colour a browser inherits reaches currentColor.
		"const color = typeof flat.color === \"string\" ? flat.color : carried;",
		"color,",
		// An attribute written for HTML is the same attribute of a drawing.
		"name.replace(/-(.)/g, (_, letter: string) => letter.toUpperCase())",
		// What belongs to the document is not part of the drawing.
		"name === \"className\" || name === \"xmlns\" || name === \"xmlnsXlink\" || name === \"aria-hidden\" || name === \"focusable\"",
		// A description is text of the document, not a shape.
		"const HIDDEN = [\"base\", \"desc\",",
	} {
		if !strings.Contains(host, wanted) {
			t.Fatalf("the drawing does not answer %q", wanted)
		}
	}
	if !strings.Contains(nativeTextModule, "export function useInheritedColor(): string | undefined") {
		t.Fatal("the box cannot hand its colour to a drawing")
	}
}

func TestNativeAnImageIsTheAssetItCameFrom(t *testing.T) {
	// Measured on the device: an <img> whose src was a module the page imported
	// and one whose src was a rooted url both painted nothing. Metro hands an
	// import over as a number, which is not a uri, and a rooted url has no
	// document here to resolve it against.
	host := nativeHost("")
	for _, wanted := range []string{
		`import { ORIGIN } from "./origin";`,
		"function imageSource(src: unknown): unknown",
		// The asset of the bundler, and the one Expo builds as an object.
		"if (typeof src === \"number\") {",
		"typeof (src as { uri?: unknown }).uri === \"string\"",
		// A rooted url is the url of the app's own server, and a
		// protocol-relative one is not.
		"if (src.charAt(0) === \"/\" && src.charAt(1) !== \"/\") {",
		"return { uri: ORIGIN + src };",
		"source={imageSource(src) as never}",
		"accessibilityLabel={alt}",
	} {
		if !strings.Contains(host, wanted) {
			t.Fatalf("the image does not answer %q", wanted)
		}
	}
}

func TestNativeADateIsTheCalendarOfTheDevice(t *testing.T) {
	// Measured on the device: a field of type="date" opened the keyboard and
	// painted the ISO string. A browser opens a calendar and hands the page
	// YYYY-MM-DD, which is what the target does with the calendar of the device.
	dependencies, _ := nativePackageManifest()["dependencies"].(map[string]any)
	if dependencies["@react-native-community/datetimepicker"] != "9.1.0" {
		t.Fatalf("the project opens a calendar without the module that opens it: %v", dependencies["@react-native-community/datetimepicker"])
	}
	host := nativeHost("")
	for _, wanted := range []string{
		`import DateTimePicker from "@react-native-community/datetimepicker";`,
		"if (props.type === \"date\" || props.type === \"time\") {",
		"function ClockField(",
		"function dateOf(value: unknown): Date | null",
		"function isoOf(date: Date): string",
		"function clockOf(mode: string, value: unknown): Date | null",
		// The event a page reads is the one of a browser.
		"target: { value: next, name, type: kind },",
		// What a browser limits, the calendar limits.
		"minimumDate={clockOf(kind, min) ?? undefined}",
		"maximumDate={clockOf(kind, max) ?? undefined}",
		// The listeners of this version: onChange is deprecated in 9.
		"onValueChange={picked}",
		"onDismiss={() => setOpen(false)}",
		// A browser shows the date of the locale, and no value is empty.
		"labelOf(kind, current)",
		"const sent = current === null ? \"\" : valueOf(kind, current);",
		// A field of a form travels with the form.
		"form.fields.set(name, sent);",
		// The dialog of Android closes itself; the spinner of iOS waits for the
		// page.
		"if (Platform.OS === \"android\") {",
		"onRequestClose={() => setOpen(false)}",
	} {
		if !strings.Contains(host, wanted) {
			t.Fatalf("the date does not answer %q", wanted)
		}
	}
}

func TestNativeATimeIsTheClockOfTheDevice(t *testing.T) {
	// Measured on the device: a field of type="time" opened the keyboard and
	// painted HH:MM as text. A browser opens a clock and hands the page HH:MM,
	// which is what the same picker does in its time mode.
	host := nativeHost("")
	for _, wanted := range []string{
		"function timeOf(value: unknown): Date | null",
		`const TIME_VALUE = /^(\d{2}):(\d{2})$/;`,
		"function hourOf(date: Date): string",
		"function valueOf(mode: string, date: Date): string",
		"return mode === \"time\" ? hourOf(date) : isoOf(date);",
		// The picker is the same one, in the mode of the field.
		"mode={kind as \"date\" | \"time\"}",
		"const kind = mode === \"time\" ? \"time\" : \"date\";",
		// The clock of the locale is what the field paints.
		"date.toLocaleTimeString([], { hour: \"2-digit\", minute: \"2-digit\" })",
		// The date that carries a clock is one the picker needs, not the page's.
		"const date = new Date();\n  date.setHours(Number(match[1]), Number(match[2]), 0, 0);",
	} {
		if !strings.Contains(host, wanted) {
			t.Fatalf("the time does not answer %q", wanted)
		}
	}
}

func TestNativeARangeIsTheSliderOfTheDevice(t *testing.T) {
	// Measured on the device: a field of type="range" was a text field of no
	// height, and the value was typed instead of dragged. A browser hands the page
	// the value as a string, and that is what the slider does.
	dependencies, _ := nativePackageManifest()["dependencies"].(map[string]any)
	if dependencies["@react-native-community/slider"] != "5.2.0" {
		t.Fatalf("the project drags a range without the module that drags it: %v", dependencies["@react-native-community/slider"])
	}
	host := nativeHost("")
	for _, wanted := range []string{
		`import Slider from "@react-native-community/slider";`,
		"if (props.type === \"range\") {",
		"function RangeField(",
		"function numberOf(value: unknown, fallback: number): number",
		// The bounds, the step and the value of a browser.
		"minimumValue={low}",
		"maximumValue={high}",
		"step={stride}",
		"const stride = step === \"any\" ? 0 : numberOf(step, 1);",
		"const middle = low + (high - low) / 2;",
		"Math.min(Math.max(wanted, low), high)",
		// The event a page reads, and the string it holds.
		"target: { value: String(next), name, type: \"range\" },",
		"for (const handler of [onInput, onChange]) {",
		"onValueChange={changed}",
		// A field of a form travels with the form.
		"form.fields.set(name, sent);",
	} {
		if !strings.Contains(host, wanted) {
			t.Fatalf("the range does not answer %q", wanted)
		}
	}
}

func TestNativeDetailsOpensAndCloses(t *testing.T) {
	// Measured on the device: a details painted the body always, while its page
	// said it was closed. A browser paints the summary as the handle and hides
	// the rest until it is opened, and that is the box that folds here.
	host := nativeHost("")
	for _, wanted := range []string{
		"NAMED.details = Details;",
		"NAMED.summary = Summary;",
		"function Details(",
		"function Summary(",
		// The state is the page's when it writes open, and the target's when it
		// does not.
		"const shown = open === undefined ? own : open === true;",
		"const [own, setOwn] = useState(defaultOpen === true);",
		// The first summary is the handle; everything else waits.
		"const head = parts.find((child) => isValidElement(child) && child.type === Summary);",
		"const body = parts.filter((child) => child !== head);",
		// The event a page reads is the one of a browser.
		"target: { open: next },",
		// A closed details does not paint its body, and the marker says where it
		// is.
		"shown ? textRuns(body, undefined) : null",
		"shown ? \"▾ \" : \"▸ \"",
		"accessibilityState={{ expanded: shown }}",
	} {
		if !strings.Contains(host, wanted) {
			t.Fatalf("the details does not answer %q", wanted)
		}
	}
}

func TestNativeAFileIsChosenFromTheDevice(t *testing.T) {
	// Measured on the device: an <input type="file"> was a text field, so there
	// was no way to choose a file. The picker is the one of the device, and the
	// form sends the upload that fetch of react-native can put in a request.
	dependencies, _ := nativePackageManifest()["dependencies"].(map[string]any)
	if dependencies["expo-document-picker"] != "~57.0.3" {
		t.Fatalf("the project chooses a file without the module that chooses it: %v", dependencies["expo-document-picker"])
	}
	host := nativeHost("")
	for _, wanted := range []string{
		`import * as DocumentPicker from "expo-document-picker";`,
		`if (props.type === "file") {`,
		"function FileField(",
		"DocumentPicker.getDocumentAsync({",
		// What the page asked for is what the picker offers.
		"type: accepted(accept),",
		"multiple: multiple === true,",
		// The file has to survive the dialog that chose it.
		"copyToCacheDirectory: true,",
		"if (result.canceled) {",
		`type: asset.mimeType ?? "application/octet-stream",`,
		// The event a page reads is the one of a browser.
		"target: { files: picked, value: said, name, type: \"file\" },",
		// A form carries the upload as the part the target's own uploader reads.
		"form.files.set(name, uploads);",
		"form.files.set(name, picked.map((one) => ({ uri: one.uri, name: one.name, type: one.type })));",
	} {
		if !strings.Contains(host, wanted) {
			t.Fatalf("the file does not answer %q", wanted)
		}
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
	if !strings.Contains(nativeScreen, `"native-html", "native-body", document.class`) {
		t.Fatal("the screen drops the document class, or the root of the stylesheet")
	}
	if !strings.Contains(nativeScreen, "{ direction }") {
		t.Fatal("the screen drops the document direction")
	}
	if !strings.Contains(nativeScreen, `document.dir === "rtl" || document.dir === "ltr"`) {
		t.Fatal("the screen forwards a direction react-native cannot take")
	}
	// The root of the stylesheet is painted whether or not the document says
	// anything: the typeface and the line height of a browser live on html and
	// body, and a screen that painted none of it painted the font of the system.
	// Measured on the device with the preflight of Tailwind expanded.
	if !strings.Contains(nativeScreen, "const root = ") {
		t.Fatal("the screen does not paint the root of the stylesheet")
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
	if _, err := writeNativeProject(native, prepared, "", filepath.Join(projectRoot, "native")); err != nil {
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
	if !strings.Contains(host, "export function nativeProps(props: unknown, tag?: string): unknown") {
		t.Fatal("the tag map does not answer the interaction of the browser")
	}
	if !strings.Contains(host, "pressed?.(shaped);") || !strings.Contains(host, "click(shaped);") {
		t.Fatal("a page that answers both names loses one of them")
	}
	if !strings.Contains(nativeJSXRuntime, "loaded().nativeProps(rest[0], typeof painted === \"string\" ? undefined : type)") {
		t.Fatal("jsx does not go through it")
	}
	if !strings.Contains(nativeReact, "host.nativeProps(props, component === undefined ? undefined : type)") {
		t.Fatal("createElement does not go through it")
	}
	if !strings.Contains(nativeReact, "return createElement(type, ...rest);") {
		t.Fatal("react-native's own views would be rewritten")
	}
}

func TestNativeAPageThatPaintsThroughAPortalStillPaints(t *testing.T) {
	// Measured on fonnso: the two buttons of the tab bar of the app -- the one that
	// opens the sidebar and the one that opens the project menu -- answered a
	// minified React error. Both paint a sheet, and a sheet of @base-ui goes
	// through react-dom's createPortal, which refuses a container that is not a
	// node: the body of this shim is an object, and the render died inside
	// FloatingPortal before anything was painted.
	//
	// What the shim has to carry for that to work is the shape of a node, not only
	// the members the page it was written for happened to ask for: with no
	// createPortal replaced the sheet threw at the container, and with a body that
	// had no contains the walk that hides the rest of the app while a dialog is
	// open (markOthers) threw "undefined is not a function" right after.
	for _, wanted := range []string{
		"nodeType: 1",
		"contains: () => false",
		"children: [],",
		"getRootNode() {",
		// A portal into the body of the page paints, and a portal into a fragment
		// does not: a component collects in a fragment the nodes it does not want
		// painted, and the items of a closed select were landing over the page.
		"dom.createPortal = (children, container) => {",
		"return null;",
	} {
		if !strings.Contains(nativeDOM, wanted) {
			t.Fatalf("the document of the shim does not carry %q", wanted)
		}
	}

	// The patch goes on react-dom itself, and a patch is one patch only if every
	// module is looking at one copy of it: the name is resolved from the workspace
	// for whoever asks. Measured on the app: two copies in the bundle, and the page
	// was calling the one that was not patched.
	if !strings.Contains(nativeMetroHelper, `if (moduleName === "react-dom") {`) {
		t.Fatal("react-dom is not resolved to a single copy")
	}
	if !strings.Contains(nativeMetroHelper, `path.join(workspaceRoot, "package.json")`) {
		t.Fatal("the copy is not the one of the workspace")
	}
}

func TestNativeCarriesTheStateOfAnElementToTheStylesheet(t *testing.T) {
	// Measured on fonnso: the sheet of the project menu of the app came out at
	// the top of the list, in the order the tree happened to have it, with none
	// of its placement applied. Every one of those rules lived under a
	// data-[side=right] variant, and a variant is only taken when the element
	// says what it is: the compiler reads that from a prop named dataSet, and an
	// attribute travelling with the name a browser gave it reached nothing.
	host := nativeHost("")
	for _, wanted := range []string{
		`if (!name.startsWith("data-") || name.length === 5) {`,
		`const key = name.slice(5).replace(/-(.)/g, (_, letter: string) => letter.toUpperCase());`,
		"state[key] = given[name];",
		"given.dataSet = state;",
		"delete given[name];",
	} {
		if !strings.Contains(host, wanted) {
			t.Fatalf("the tag map does not carry the state of an element: %q", wanted)
		}
	}
	// The rest of the shape is what react-native-css reads, and the name is the
	// one the compiler spells: a dash and the letter after it, in camel case.
	if !strings.Contains(host, "const state = (given.dataSet ?? {}) as Record<string, unknown>;") {
		t.Fatal("a dataSet the page wrote itself would be thrown away")
	}
}

func TestNativeLeavesTheProjectAloneWhenNothingChanged(t *testing.T) {
	// Metro watches the generated project and a file replaced with the same bytes
	// is a change for the watcher: the bundler throws its cache away for it.
	// Measured on the app of the case: `bifrost native` with nothing changed cost a
	// cold bundle of 81.6 s, and the only reason the run could not be free was that
	// every file of the project was replaced -- `os.RemoveAll` of `app/` and
	// `bifrost/` and then a write of each one: measured, 38 of the 44 files of the
	// generated project, and only one of them had any reason to change.
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
	overlay := filepath.Join(projectRoot, "native")
	if err := os.MkdirAll(overlay, 0o755); err != nil {
		t.Fatal(err)
	}
	// An app that ships a file of its own under a tree the generator owns is the
	// case that was left behind: `metro.config.js` was written by the generator and
	// then replaced by the app's, so its mtime changed on every run even after the
	// strays were taken care of. Measured on the app of the case, with that write
	// left in: it was the only file of the run that changed nothing.
	if err := os.WriteFile(filepath.Join(overlay, "metro.config.js"), []byte("module.exports = withBifrostWarnings({});\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := writeNativeProject(native, prepared, "", overlay); err != nil {
		t.Fatal(err)
	}
	before := map[string]time.Time{}
	for _, tree := range []string{"app", "bifrost", "metro.config.js"} {
		err := filepath.WalkDir(filepath.Join(native, tree), func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			before[path] = info.ModTime()
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(before) < 20 {
		t.Fatalf("the project is too small to say anything: %d files", len(before))
	}
	time.Sleep(20 * time.Millisecond)
	same, err := writeNativeProject(native, prepared, "", overlay)
	if err != nil {
		t.Fatal(err)
	}
	if !same {
		t.Fatal("a run that changes nothing does not say so")
	}
	for path, when := range before {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("%s went missing: %v", path, err)
		}
		if !info.ModTime().Equal(when) {
			t.Fatalf("%s was written again, which is a cold bundle for Metro", path)
		}
	}
	// And the run that does change something says that too, or the line above is
	// worth nothing: a route added is a file added.
	if err := os.MkdirAll(filepath.Join(appRoot, "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appRoot, "other", "page.tsx"), []byte("export default function Other() { return null; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prepared, err = prepareAppRouter(t.Context(), projectRoot, appRoot, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	same, err = writeNativeProject(native, prepared, "", overlay)
	if err != nil {
		t.Fatal(err)
	}
	if same {
		t.Fatal("a run that added a screen says nothing changed")
	}
	// And what the app deleted does not stay: the tree is a build artifact.
	if err := os.RemoveAll(filepath.Join(appRoot, "other")); err != nil {
		t.Fatal(err)
	}
	prepared, err = prepareAppRouter(t.Context(), projectRoot, appRoot, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeNativeProject(native, prepared, "", overlay); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(native, "app", "other.tsx")); !os.IsNotExist(err) {
		t.Fatalf("a removed route left its screen behind: %v", err)
	}
}

func TestNativeKeepsTheBoxesOfAFlexContainerApart(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	// A block that holds a string becomes a Text, because only a Text may hold
	// one. Which strings are one text and which are boxes of their own is what a
	// browser decides from what lays the children out: in a block container the
	// children flow as text, and in a flex container every child is an item.
	//
	// Measured on the app: the footer of the landing page is a flex row with
	// justify-between around two spans, and it came out as the single line
	// "Fonnso3 Lines Studio" -- the two items merged into one text, with nowhere
	// for the space between them to go. The same rule is what a sheet header needs.
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
	if _, err := writeNativeProject(native, prepared, "http://example.test", filepath.Join(projectRoot, "native")); err != nil {
		t.Fatal(err)
	}
	// The elements of the map that are a run of text are the ones the probe calls
	// inline, and that is all runsOf asks of its caller.
	probe := `import { runsOf, asksForFlex } from "./bifrost/runs.js";
const isInline = (child) => typeof child === "object" && child !== null && child.inline === true;
const nameOf = (child) => (typeof child === "object" && child !== null ? child.name : String(child));
// A run of text comes back as the element the caller asked for, and a child that
// is not a run comes back as it is: the probe names both so the two can be told
// apart in one string.
const wrap = (run, at) => ({ text: true, value: run.map(nameOf).join("") });
const label = (run) => (run && run.text ? "text(" + run.value + ")" : nameOf(run));
const shape = (children, className) => JSON.stringify(runsOf(children, className, isInline, wrap).map(label));
const span = (name) => ({ inline: true, name });
const box = (name) => ({ inline: false, name });

const cases = [
  // A block container flows its children as text, so a bold word in the middle of
  // a sentence is one line.
  ["block with two spans", [span("a"), span("b")], "", '["text(ab)"]'],
  ["block with text around an element", ["hola ", span("b"), " mundo"], "", '["text(hola b mundo)"]'],
  // A flex container makes an item of each child, so the two spans are two boxes.
  // The element is already a component, so it travels as itself: what changes is
  // that it does not join the run of text next to it.
  ["flex with two spans", [span("a"), span("b")], "flex items-center justify-between", '["a","b"]'],
  ["inline-flex with two spans", [span("a"), span("b")], "inline-flex", '["a","b"]'],
  // A run of bare text is one item and an element is another.
  ["flex with text around an element", ["hola ", span("b")], "flex", '["text(hola )","b"]'],
  // Anything that is not a run of text keeps its own component either way.
  ["a block child always breaks the run", ["hola", box("x"), "chau"], "", '["text(hola)","x","text(chau)"]'],
  ["the same, in a flex container", ["hola", box("x")], "flex", '["text(hola)","x"]'],
  // A class that is not a display does not make a container: flex-1 says how much
  // of the box an item takes, and it lays out nothing.
  ["flex-1 is not a flex container", [span("a"), span("b")], "flex-1", '["text(ab)"]'],
  // A variant is a class with a colon in it, and this does not read behind it.
  ["a variant is not the class", [span("a"), span("b")], "md:flex", '["text(ab)"]'],
];

let bad = 0;
for (const [what, children, className, want] of cases) {
  const got = shape(children, className);
  if (got !== want) {
    console.log("MAL " + what + ": " + got + " (esperaba " + want + ")");
    bad++;
  }
}
for (const [className, want] of [["flex", true], ["inline-flex", true], ["flex flex-col", true], ["flex-1", false], ["", false], [undefined, false], ["md:flex", false]]) {
  const got = asksForFlex(className);
  if (got !== want) {
    console.log("MAL asksForFlex(" + className + "): " + got);
    bad++;
  }
}
console.log(bad === 0 ? "todos bien" : bad + " mal");
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
		t.Fatalf("runsOf failed: %v\n%s", err, output)
	}
	if said := strings.TrimSpace(string(output)); said != "todos bien" {
		t.Fatal(said)
	}
}

// nativeFunctionBody reads one function out of the generated map, so a test can
// ask about the shape of a component the app is written against and not about a
// copy of it.
func nativeFunctionBody(source, name string) (string, error) {
	opening := "function " + name + "("
	start := strings.Index(source, opening)
	if start < 0 {
		return "", fmt.Errorf("the map has no function %s", name)
	}
	rest := source[start:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		return "", fmt.Errorf("%s is not closed", name)
	}
	return rest[:end], nil
}

func TestNativePaintsTheChildrenOfAButtonAsItemsOfTheirOwn(t *testing.T) {
	// A button and a link are flex containers in a browser, so what is inside
	// them is items: an icon is one and the label is another. With the whole of it
	// in one Text the icon takes the width of the box and the label goes after it,
	// out of the button -- measured on the login page of fonnso, the mark of
	// Google at [84,1097][819,1197] and the label painted from x=834 inside a
	// button that ends at 996, which is the label painted past its border.
	//
	// The class is what says which of the two shapes it is: an icon of a button
	// inside a flex container is an item, and one inside a block would be part of
	// the run. That is why the class is handed to the rule and not just to the
	// view.
	host := nativeHost("")
	for _, name := range []string{"Box", "Anchor", "Button"} {
		body, err := nativeFunctionBody(host, name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(body, "textRuns(children, className)") {
			t.Errorf("%s does not ask the run rule for its children:\n%s", name, body)
		}
	}
	// One place decides it, and the thing that decides it is the class: a rule
	// copied into every component is a rule that drifts.
	if count := strings.Count(host, "runsOf("); count != 1 {
		t.Errorf("runsOf is asked %d times, want once", count)
	}
	body, err := nativeFunctionBody(host, "textRuns")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "className,") {
		t.Errorf("textRuns does not hand the class to the rule:\n%s", body)
	}
}

func TestNativeShipsTheFontsTheAppDeclares(t *testing.T) {
	// A native screen picks a font by family and loads it as a file, so a family
	// with four weights is four families here. The app declares them once, and
	// the target loads them, folds the weight a rule asked for into the name, and
	// hands a box's text down to the text inside it, which is what a browser does
	// and what a view does not.
	projectRoot := t.TempDir()
	appRoot := filepath.Join(projectRoot, "app")
	if err := os.MkdirAll(appRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appRoot, "page.tsx"), []byte("export default function Page() { return null; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(projectRoot, "native")
	fonts := `[{"fontFamily":"Plus Jakarta Sans Variable","fonts":[
		{"path":"assets/fonts/jakarta-600.ttf","weight":600},
		{"path":"assets/fonts/jakarta-400.ttf","weight":400}
	]}]`
	for name, content := range map[string]string{
		"fonts.json":                   fonts,
		"assets/fonts/jakarta-400.ttf": "400",
		"assets/fonts/jakarta-600.ttf": "600",
	} {
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
	if _, err := writeNativeProject(native, prepared, "", overlay); err != nil {
		t.Fatal(err)
	}

	// The file the layout loads: one name per weight, and the weights of each
	// family so that a rule finds the file it asked for.
	module, err := os.ReadFile(filepath.Join(native, "bifrost", "fonts.ts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"Plus Jakarta Sans Variable": [400, 600],`,
		`"Plus Jakarta Sans Variable 400": require("../assets/fonts/jakarta-400.ttf"),`,
		`"Plus Jakarta Sans Variable 600": require("../assets/fonts/jakarta-600.ttf"),`,
	} {
		if !strings.Contains(string(module), want) {
			t.Errorf("fonts.ts does not have %s:\n%s", want, module)
		}
	}

	// The layout loads them before it paints, and an app that declares none is in
	// on the first frame.
	layout, err := os.ReadFile(filepath.Join(native, "app", "_layout.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(layout), `import { useFonts } from "expo-font";`) {
		t.Error("the layout does not load the fonts of the app")
	}
	if !strings.Contains(string(layout), "const [ready] = useFonts(FONTS);") || !strings.Contains(string(layout), "if (!NONE && !ready)") {
		t.Error("the layout paints a frame before the fonts of the app are in")
	}

	// The tags of the map are painted by the wrappers that keep what a browser
	// inherits and fold the weight into the family.
	host, err := os.ReadFile(filepath.Join(native, "bifrost", "host.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(host), `import { StyledAnimatedView as AnimatedView, StyledPressable as Pressable, StyledStickyView as StickyView, StyledText as Text, StyledTextInput as TextInput, StyledView as View, useInheritedColor } from "./text";`) {
		t.Error("the tag map paints plain views, so nothing inherits and no weight reaches a file")
	}
	text, err := os.ReadFile(filepath.Join(native, "bifrost", "text.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"inheritedText(carried, style)", "textStyle(own, families)", "<Carried.Provider value={own}>"} {
		if !strings.Contains(string(text), want) {
			t.Errorf("text.tsx does not have %q:\n%s", want, text)
		}
	}
	style, err := os.ReadFile(filepath.Join(native, "bifrost", "text-style.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(style), "module.exports = { inheritedText, textStyle };") {
		t.Errorf("the pass is not shipped whole:\n%s", style)
	}

	// The module the layout loads needs its package.
	manifest, err := os.ReadFile(filepath.Join(native, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), `"expo-font"`) {
		t.Error("the project loads fonts without the module that loads them")
	}
}

func TestNativeRefusesAFontTheOverlayDoesNotShip(t *testing.T) {
	cases := []struct {
		name  string
		fonts string
		want  string
	}{
		{
			"a file the overlay does not ship",
			`[{"fontFamily":"A","fonts":[{"path":"assets/a-400.ttf","weight":400}]}]`,
			"the overlay does not ship assets/a-400.ttf",
		},
		{
			"a weight of zero",
			`[{"fontFamily":"A","fonts":[{"path":"assets/a.ttf","weight":0}]}]`,
			"no path or no weight",
		},
		{
			"the same weight twice",
			`[{"fontFamily":"A","fonts":[{"path":"assets/a.ttf","weight":400},{"path":"assets/a.ttf","weight":400}]}]`,
			"names weight 400 twice",
		},
		{
			"a family with no files",
			`[{"fontFamily":"A","fonts":[]}]`,
			"a family has no name or no files",
		},
		{
			"what is not JSON",
			`{`,
			"fonts.json",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			overlay := t.TempDir()
			if err := os.MkdirAll(filepath.Join(overlay, "assets"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(overlay, "assets", "a.ttf"), []byte("A"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(overlay, "fonts.json"), []byte(test.fonts), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := nativeFonts(overlay)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err = %v, want one that says %q", err, test.want)
			}
		})
	}
}

func TestNativeAnAppWithoutFontsDeclaresAnEmptyMap(t *testing.T) {
	module := nativeFontsModule(nil)
	if !strings.Contains(module, "export const families: Record<string, number[]> = {\n};") {
		t.Errorf("a project without fonts does not declare an empty map:\n%s", module)
	}
	if !strings.Contains(module, "export const fonts: Record<string, number> = {\n};") {
		t.Errorf("a project without fonts does not declare an empty map:\n%s", module)
	}
}
