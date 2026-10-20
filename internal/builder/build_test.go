package builder

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

func TestPlanViewsDeduplicatesSharedViews(t *testing.T) {
	describe := protocol.DescribeResult{Spec: protocol.Spec{Routes: []protocol.RouteSpec{
		{Pattern: "/a", View: "pages/shared.tsx", Kind: "server"},
		{Pattern: "/b", View: "pages/shared.tsx", Kind: "static"},
		{Pattern: "/client", View: "pages/shared.tsx", Kind: "client"},
	}}}
	plans, routes, _, err := planViews(describe, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(plans), 2; got != want {
		t.Fatalf("plans = %d, want %d", got, want)
	}
	if routes["/a"] != routes["/b"] {
		t.Fatal("server and static routes did not share hydrate view")
	}
	if routes["/a"] == routes["/client"] {
		t.Fatal("hydrate and mount views shared an ID")
	}
}

func TestCollectExternalDevelopmentViewsUsesGeneratedEntries(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "entries", "client.tsx")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("export {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := viewPlan{ID: digest("view"), Source: "pages/app.tsx", Mode: "mount", ClientFile: entry}
	views, files, err := collectExternalDevelopmentViews(root, []viewPlan{plan})
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || len(files) != 1 || views[0].Client.Entry != files[0] {
		t.Fatalf("views = %+v files = %+v", views, files)
	}
	if files[0].Path != "entries/client.tsx" {
		t.Fatalf("entry path = %q", files[0].Path)
	}
}

func TestViteManifestDependencyTraversal(t *testing.T) {
	manifest := viteManifest{
		"entry.tsx": {File: "assets/entry.js", Name: "entry", IsEntry: true, Imports: []string{"shared.js"}, CSS: []string{"assets/entry.css"}},
		"shared.js": {File: "assets/shared.js", Imports: []string{"base.js"}, CSS: []string{"assets/shared.css"}},
		"base.js":   {File: "assets/base.js"},
	}
	key, _, err := viteEntry(manifest, "entry")
	if err != nil {
		t.Fatal(err)
	}
	imports, styles := viteClientDependencies(manifest, key)
	if strings.Join(imports, ",") != "assets/shared.js,assets/base.js" {
		t.Fatalf("imports = %v", imports)
	}
	if strings.Join(styles, ",") != "assets/entry.css,assets/shared.css" {
		t.Fatalf("styles = %v", styles)
	}
}

func TestFileRefsMatch(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "entry.tsx")
	if err := os.WriteFile(filePath, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	ref, err := fileRef(root, filePath)
	if err != nil {
		t.Fatal(err)
	}
	if !fileRefsMatch(root, []protocol.FileRef{ref}) {
		t.Fatal("matching file was rejected")
	}
	if err := os.WriteFile(filePath, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fileRefsMatch(root, []protocol.FileRef{ref}) {
		t.Fatal("changed file was accepted")
	}
}

func TestSupportedToolVersions(t *testing.T) {
	if !supportedBunVersion("1.3.14") || supportedBunVersion("1.2.9") || supportedBunVersion("2.0.0") {
		t.Fatal("unexpected Bun version policy")
	}
	if !supportedViteVersion("8.2.1") || supportedViteVersion("7.0.0") {
		t.Fatal("unexpected Vite version policy")
	}
	if majorMinorVersion("19.2.4") != "19.2" || majorMinorVersion("bad") != "" {
		t.Fatal("unexpected React version parsing")
	}
}

func TestReadPackageVersionWalksMonorepoParents(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "apps", "site")
	packageDir := filepath.Join(root, "node_modules", "vite")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "package.json"), []byte(`{"version":"8.2.1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readPackageVersion(app, "vite"); got != "8.2.1" {
		t.Fatalf("version = %q", got)
	}
}

func TestCopyPublicRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "public"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "public", "secret")); err != nil {
		t.Fatal(err)
	}
	if _, err := copyPublic(root, t.TempDir()); err == nil || !strings.Contains(err.Error(), "public symlink") {
		t.Fatalf("copy error = %v", err)
	}
}

func TestCopyPublicRejectsServeMuxMetacharacters(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "public"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "public", "page{x}.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := copyPublic(root, t.TempDir()); err == nil || !strings.Contains(err.Error(), "URL escaping") {
		t.Fatalf("copy error = %v", err)
	}
}

func TestResolveSourceFileRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "page.tsx"), []byte("export function Page() {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "pages")); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveSourceFile(root, "pages/page.tsx"); err == nil || !strings.Contains(err.Error(), "escapes source root") {
		t.Fatalf("resolve error = %v", err)
	}
}

func TestGeneratedEmbedSupportsChildOutputDirectory(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "generated", "web")
	if err := ensureGeneratedEmbed(root, "main", output, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "zz_bifrost_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "//go:embed all:generated/web") || !strings.Contains(string(data), `fs.Sub(bifrostEmbedded, "generated/web")`) {
		t.Fatalf("bad generated embed:\n%s", data)
	}
}

func TestGeneratedEmbedForBuildPhasesOnlyEmbedsThePlaceholder(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "generated", "web")
	if err := ensureGeneratedEmbed(root, "main", output, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "zz_bifrost_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "//go:embed generated/web/embed.placeholder") || strings.Contains(string(data), "all:") {
		t.Fatalf("bad phase embed:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(output, "embed.placeholder")); err != nil {
		t.Fatal(err)
	}
}

func TestWriteEntriesPassesAbortSignalToReact(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "entries"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "pages"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pages", "app.tsx"), []byte("export function Page() {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := viewPlan{
		ID:         digest("view"),
		Source:     "pages/app.tsx",
		Mode:       "hydrate",
		ClientFile: filepath.Join(root, "entries", "client.tsx"),
		ServerFile: filepath.Join(root, "entries", "server.tsx"),
	}
	if err := writeEntries(root, root, []viewPlan{plan}, nil, ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(plan.ServerFile)
	if err != nil {
		t.Fatal(err)
	}
	generated := string(data)
	if !strings.Contains(generated, "render(props, signal)") || !strings.Contains(generated, "{ signal, onError") {
		t.Fatalf("generated SSR entry does not propagate cancellation:\n%s", generated)
	}
}

func TestWriteEntriesAddsReloadOnlyInDevelopment(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "entries"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "pages"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pages", "app.tsx"), []byte("export function Page() {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := viewPlan{
		ID:         digest("view"),
		Source:     "pages/app.tsx",
		Mode:       "mount",
		ClientFile: filepath.Join(root, "entries", "client.tsx"),
	}
	if err := writeEntries(root, root, []viewPlan{plan}, nil, digest("build")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(plan.ClientFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "/_bifrost/build-id") {
		t.Fatal("development entry has no reload polling")
	}

	if err := writeEntries(root, root, []viewPlan{plan}, nil, ""); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(plan.ClientFile)
	if strings.Contains(string(data), "/_bifrost/build-id") {
		t.Fatal("production entry contains reload polling")
	}
}

func TestFrontendBuildErrorKeepsTheEndOfWhatTheRunnerSaid(t *testing.T) {
	command := exec.Command("bun", "run", "vite-runner.ts")
	log := "vite v8.0.0 building for production\n" +
		"error: Could not resolve \"../../app/probe/page.tsx\" from .bifrost/views/page-2.tsx\n"
	said := frontendBuildError(command, errors.New("exit status 1"), log)
	if !strings.Contains(said.Error(), "Could not resolve") {
		t.Fatalf("the failure does not carry what the runner said:\n%s", said)
	}
	if !strings.Contains(said.Error(), "vite-runner.ts") {
		t.Fatalf("the failure does not name the runner:\n%s", said)
	}
	if quiet := frontendBuildError(command, errors.New("exit status 1"), ""); strings.Contains(quiet.Error(), "\n") {
		t.Fatalf("a runner that said nothing left an empty tail:\n%s", quiet)
	}
}

func TestFrontendBuildErrorDropsRunnerNoise(t *testing.T) {
	command := exec.Command("bun", "run", "vite-runner.ts")
	said := "\x1b[33m[WARN] internal\x1b[0m\n" +
		"✗ Build failed in 38ms\n" +
		"44 | summary += err;\n" +
		"\n" +
		"error: Build failed with 1 error:\n" +
		"\n" +
		"\x1b[31m[builtin:vite-transform]\x1b[0m Expected `:` but found `<`\n" +
		"   ╭─[ app/page.tsx:1:40 ]\n" +
		" 1 │ export default function Page( {\n" +
		"\n" +
		"      at aggregate (/tmp/rolldown.mjs:49:22)\n" +
		"Bun v1.4.3 (Linux x64)\n"
	failure := frontendBuildError(command, errors.New("exit status 1"), said)
	for _, unwanted := range []string{"\x1b", "summary +=", "at aggregate", "Bun v"} {
		if strings.Contains(failure.Error(), unwanted) {
			t.Fatalf("the failure kept %q:\n%s", unwanted, failure)
		}
	}
	for _, wanted := range []string{"Expected", "app/page.tsx", "vite-runner.ts"} {
		if !strings.Contains(failure.Error(), wanted) {
			t.Fatalf("the failure lost %q:\n%s", wanted, failure)
		}
	}
}

func TestFrontendWarningsKeepsOnlyUserWarnings(t *testing.T) {
	said := "\x1b[33m[INEFFECTIVE_DYNAMIC_IMPORT]\x1b[0m .bifrost/views/page-0.tsx is imported twice\n\n" +
		"[IMPORT_IS_UNDEFINED] Import `renderHead` is undefined in '.bifrost/views/page-0.tsx'\n" +
		"   ╭─[ .bifrost/app/server.tsx:6:29 ]\n\n" +
		"\x1b[33m[UNUSED]\x1b[0m app/page.tsx is unused\n"
	warnings := frontendWarnings(said)
	if strings.Contains(warnings, ".bifrost/") || strings.Contains(warnings, "\x1b") {
		t.Fatalf("the warnings kept generated files or ANSI:\n%s", warnings)
	}
	if !strings.Contains(warnings, "app/page.tsx is unused") {
		t.Fatalf("the warnings lost the user's warning:\n%s", warnings)
	}
}

func TestLastLinesKeepsTheEndOfTheLog(t *testing.T) {
	var log strings.Builder
	for index := 0; index < 100; index++ {
		fmt.Fprintf(&log, "line %d\n", index)
	}
	kept := lastLines(log.String(), 40)
	lines := strings.Split(kept, "\n")
	if len(lines) != 40 {
		t.Fatalf("kept %d lines, want 40:\n%s", len(lines), kept)
	}
	if lines[0] != "line 60" || lines[39] != "line 99" {
		t.Fatalf("the end of the log is not what was kept:\n%s", kept)
	}
	if lastLines("only one line\n", 40) != "only one line" {
		t.Fatal("a log of one line lost it")
	}
	if lastLines("", 40) != "" {
		t.Fatal("an empty log is not empty")
	}
}

func TestPlanViewsSkipsHTMLViews(t *testing.T) {
	describe := protocol.DescribeResult{Spec: protocol.Spec{Routes: []protocol.RouteSpec{
		{Pattern: "/", View: "app/page.html", Kind: "server", Layouts: []string{"app/layout.html"}},
	}}}
	plans, routes, _, err := planViews(describe, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 0 {
		t.Fatalf("html view produced %d frontend plans", len(plans))
	}
	if routes["/"] != templateViewID(describe.Spec.Routes[0]) {
		t.Fatal("html route view id was not registered")
	}
}

func TestCollectHTMLViews(t *testing.T) {
	root := t.TempDir()
	for name, source := range map[string]string{
		"app/page.html":   "<h1>{{.Props.title}}</h1>",
		"app/layout.html": "<main>{{.Content}}</main>",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	spec := protocol.Spec{Routes: []protocol.RouteSpec{
		{Pattern: "/", View: "app/page.html", Kind: "server", Layouts: []string{"app/layout.html"}},
	}}
	routeViews := map[string]string{"/": templateViewID(spec.Routes[0])}
	views, err := collectHTMLViews(root, spec, routeViews)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 {
		t.Fatalf("views = %d", len(views))
	}
	view := views[0]
	if view.Mode != "html" || view.Source != "app/page.html" || view.HTML == nil {
		t.Fatalf("view = %+v", view)
	}
	if view.HTML.Page.Source != "<h1>{{.Props.title}}</h1>" {
		t.Fatalf("page source = %q", view.HTML.Page.Source)
	}
	if len(view.HTML.Layouts) != 1 || view.HTML.Layouts[0].Path != "app/layout.html" {
		t.Fatalf("layouts = %+v", view.HTML.Layouts)
	}
}

func TestPlanViewsBuildsScriptEntry(t *testing.T) {
	describe := protocol.DescribeResult{Spec: protocol.Spec{Routes: []protocol.RouteSpec{
		{Pattern: "/", View: "app/page.html", Kind: "server", Layouts: []string{"app/layout.html"}, Script: "app/page.js"},
	}}}
	plans, routes, _, err := planViews(describe, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 {
		t.Fatalf("plans = %d", len(plans))
	}
	plan := plans[0]
	if plan.Mode != "script" || plan.Source != "app/page.js" || plan.ViewSource != "app/page.html" {
		t.Fatalf("plan = %+v", plan)
	}
	if routes["/"] != plan.ID {
		t.Fatal("html route view id was not registered")
	}
}

func TestWriteEntriesImportsScript(t *testing.T) {
	sourceRoot := t.TempDir()
	output := t.TempDir()
	script := filepath.Join(sourceRoot, "app", "page.js")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("console.log('page')"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := viewPlan{
		ID:         digest("script"),
		Source:     "app/page.js",
		ViewSource: "app/page.html",
		Mode:       "script",
		ClientFile: filepath.Join(output, "entries", "script-client.tsx"),
	}
	if err := os.MkdirAll(filepath.Dir(plan.ClientFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeEntries(sourceRoot, output, []viewPlan{plan}, nil, ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(plan.ClientFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "import "+strconv.Quote(filepath.ToSlash(script))+";") {
		t.Fatalf("script entry = %q", data)
	}
	if strings.Contains(string(data), "react-dom") {
		t.Fatalf("script entry contains React: %q", data)
	}
}

func TestAttachHTMLViewsKeepsClientAssets(t *testing.T) {
	entry := protocol.FileRef{Path: "dist/app.js", Hash: "abc", Size: 1}
	views := []protocol.BuiltView{{ID: "view", Source: "app/page.html", Mode: "html", Client: protocol.AssetSet{Entry: entry}}}
	htmlViews := []protocol.BuiltView{{ID: "view", Source: "app/page.html", Mode: "html", HTML: &protocol.HTMLAssets{Page: protocol.TemplateSource{Path: "app/page.html", Source: "<h1>x</h1>"}}}}
	merged := attachHTMLViews(views, htmlViews)
	if len(merged) != 1 {
		t.Fatalf("views = %d", len(merged))
	}
	if merged[0].Client.Entry != entry || merged[0].HTML == nil {
		t.Fatalf("view = %+v", merged[0])
	}
}
