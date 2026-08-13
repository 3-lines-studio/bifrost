package builder

import (
	"os"
	"path/filepath"
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
	plans, routes, err := planViews(describe, t.TempDir())
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
	if err := ensureGeneratedEmbed(root, "main", output); err != nil {
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
	if err := writeEntries(root, root, []viewPlan{plan}, ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(plan.ServerFile)
	if err != nil {
		t.Fatal(err)
	}
	generated := string(data)
	if !strings.Contains(generated, "render(props, signal)") || !strings.Contains(generated, "{ signal }") {
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
	if err := writeEntries(root, root, []viewPlan{plan}, digest("build")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(plan.ClientFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "/_bifrost/build-id") {
		t.Fatal("development entry has no reload polling")
	}

	if err := writeEntries(root, root, []viewPlan{plan}, ""); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(plan.ClientFile)
	if strings.Contains(string(data), "/_bifrost/build-id") {
		t.Fatal("production entry contains reload polling")
	}
}
