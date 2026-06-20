package usecase

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/3-lines-studio/bifrost/internal/core"
)

type fakeRenderer struct {
	buildCalls           int
	buildSSRCalls        int
	buildSSRBatchSizes   []int
	individualBuildCalls int
	renderCalls          int
	buildFn              func(entrypoints []string, outdir string, entryNames []string, framework string) (map[string]core.ClientBuildResult, error)
	buildSSRFn           func(entrypoints []string, outdir string, framework string) error
	renderFn             func(componentPath string, props any) (core.RenderedPage, error)
}

func (f *fakeRenderer) Render(componentPath string, props any) (core.RenderedPage, error) {
	f.renderCalls++
	if f.renderFn != nil {
		return f.renderFn(componentPath, props)
	}
	return core.RenderedPage{}, nil
}

func (f *fakeRenderer) Build(entrypoints []string, outdir string, entryNames []string, framework string) (map[string]core.ClientBuildResult, error) {
	f.buildCalls++
	if len(entryNames) == 1 {
		f.individualBuildCalls++
	}
	if f.buildFn != nil {
		return f.buildFn(entrypoints, outdir, entryNames, framework)
	}
	return map[string]core.ClientBuildResult{}, nil
}

func (f *fakeRenderer) BuildSSR(entrypoints []string, outdir string, framework string) error {
	f.buildSSRCalls++
	f.buildSSRBatchSizes = append(f.buildSSRBatchSizes, len(entrypoints))
	if f.buildSSRFn != nil {
		return f.buildSSRFn(entrypoints, outdir, framework)
	}
	return nil
}

func TestPageServiceDevSSRBuildsThenRenders(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "pages", "home.tsx"), "export default function Page(){ return <div>Hello</div> }")

	renderer := &fakeRenderer{
		buildSSRFn: func(entrypoints []string, outdir string, framework string) error {
			name := strings.TrimSuffix(filepath.Base(entrypoints[0]), filepath.Ext(entrypoints[0]))
			writeTestFile(t, filepath.Join(outdir, name+".js"), "// ssr")
			return nil
		},
		renderFn: func(componentPath string, props any) (core.RenderedPage, error) {
			if componentPath == "" {
				t.Fatal("expected render path")
			}
			return core.RenderedPage{Head: "<title>Home</title>", Body: "<div>Hello</div>"}, nil
		},
	}
	service := NewPageService(renderer, nil, nil)

	restore := chdirForTest(t, tmpDir)
	defer restore()

	input := ServePageInput{
		Config: core.PageConfig{
			ComponentPath: "./pages/home.tsx",
			Mode:          core.ModeSSR,
		},
		DefaultHTMLLang: "en",
		IsDev:           true,
		EntryName:       core.EntryNameForPath("./pages/home.tsx"),
		RequestPath:     "/",
		Request:         httptest.NewRequest(http.MethodGet, "/", nil),
	}

	output := service.ServePage(context.Background(), input)
	if output.Error != nil {
		t.Fatalf("ServePage() error = %v", output.Error)
	}
	if output.Action != core.ActionRenderSSR {
		t.Fatalf("ServePage() action = %v", output.Action)
	}
	if output.HTML == "" {
		t.Fatal("expected HTML output")
	}
	if !strings.Contains(output.HTML, "<div>Hello</div>") {
		t.Fatalf("expected rendered body, got %q", output.HTML)
	}
	if !strings.Contains(output.HTML, "<title>Home</title>") {
		t.Fatalf("expected head, got %q", output.HTML)
	}
	if renderer.buildCalls != 1 || renderer.buildSSRCalls != 1 {
		t.Fatalf("expected one dev setup build, got Build=%d BuildSSR=%d", renderer.buildCalls, renderer.buildSSRCalls)
	}
}

func TestPageServiceStaticPrerenderReturnsNotFoundForMissingPath(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "pages", "blog.tsx"), "export default function Page(){ return <div>Blog</div> }")

	renderer := &fakeRenderer{
		buildSSRFn: func(entrypoints []string, outdir string, framework string) error {
			name := strings.TrimSuffix(filepath.Base(entrypoints[0]), filepath.Ext(entrypoints[0]))
			writeTestFile(t, filepath.Join(outdir, name+".js"), "// ssr")
			return nil
		},
	}
	service := NewPageService(renderer, nil, nil)

	restore := chdirForTest(t, tmpDir)
	defer restore()

	input := ServePageInput{
		Config: core.PageConfig{
			ComponentPath: "./pages/blog.tsx",
			Mode:          core.ModeStaticPrerender,
			StaticDataLoader: func(context.Context) ([]core.StaticPathData, error) {
				return []core.StaticPathData{{Path: "/blog/hello", Props: map[string]any{"slug": "hello"}}}, nil
			},
		},
		DefaultHTMLLang: "en",
		IsDev:           true,
		EntryName:       core.EntryNameForPath("./pages/blog.tsx"),
		RequestPath:     "/blog/missing",
		Request:         httptest.NewRequest(http.MethodGet, "/blog/missing", nil),
	}

	output := service.ServePage(context.Background(), input)
	if output.Error != nil {
		t.Fatalf("ServePage() error = %v", output.Error)
	}
	if output.Action != core.ActionNotFound {
		t.Fatalf("ServePage() action = %v", output.Action)
	}
}

func TestBuildProjectFallsBackToPerPageClientBuilds(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "main.go"), `package main
func main() {
	_ = Page("/", "./pages/home.tsx", WithClient())
	_ = Page("/about", "./pages/about.tsx", WithClient())
}`)
	writeTestFile(t, filepath.Join(tmpDir, "pages", "home.tsx"), "<title>Home</title>")
	writeTestFile(t, filepath.Join(tmpDir, "pages", "about.tsx"), "<title>About</title>")

	renderer := &fakeRenderer{
		buildFn: func(entrypoints []string, outdir string, entryNames []string, framework string) (map[string]core.ClientBuildResult, error) {
			if len(entryNames) > 1 {
				return nil, errors.New("batch failed")
			}
			name := entryNames[0]
			return map[string]core.ClientBuildResult{
				name: {
					Script: "/dist/" + name + ".js",
				},
			}, nil
		},
	}
	service := NewBuildService(renderer, nil, &mockCLIOutput{}, nil)

	result := service.BuildProject(context.Background(), BuildInput{
		MainFile:    filepath.Join(tmpDir, "main.go"),
		OriginalCwd: tmpDir,
	})
	if result.Error != nil {
		t.Fatalf("BuildProject() error = %v", result.Error)
	}
	if !result.Success {
		t.Fatal("expected build success")
	}
	if renderer.buildCalls != 3 {
		t.Fatalf("expected one batch build and two individual builds, got %d", renderer.buildCalls)
	}
	if renderer.individualBuildCalls != 2 {
		t.Fatalf("expected two individual builds, got %d", renderer.individualBuildCalls)
	}

	manifestPath := filepath.Join(tmpDir, ".bifrost", "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifest := string(data)
	if !strings.Contains(manifest, `"html": "/pages/pages-home-entry.html"`) {
		t.Fatalf("expected home html in manifest, got %s", manifest)
	}
	if !strings.Contains(manifest, `"html": "/pages/pages-about-entry.html"`) {
		t.Fatalf("expected about html in manifest, got %s", manifest)
	}
}

func TestBuildProjectCleansGeneratedDirsButPreservesBifrostRoot(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "main.go"), `package main
func main() {
	_ = Page("/", "./pages/home.tsx", WithClient())
}`)
	writeTestFile(t, filepath.Join(tmpDir, "pages", "home.tsx"), "<title>Home</title>")
	writeTestFile(t, filepath.Join(tmpDir, ".bifrost", ".gitkeep"), "keep")
	writeTestFile(t, filepath.Join(tmpDir, ".bifrost", "dist", "stale.js"), "stale")
	writeTestFile(t, filepath.Join(tmpDir, ".bifrost", "ssr", "stale.js"), "stale")
	writeTestFile(t, filepath.Join(tmpDir, ".bifrost", "entries", "stale.tsx"), "stale")
	writeTestFile(t, filepath.Join(tmpDir, ".bifrost", "pages", "stale.html"), "stale")
	writeTestFile(t, filepath.Join(tmpDir, ".bifrost", "runtime", "stale-bin"), "stale")
	writeTestFile(t, filepath.Join(tmpDir, ".bifrost", "public", "stale.txt"), "stale")

	renderer := &fakeRenderer{
		buildFn: func(entrypoints []string, outdir string, entryNames []string, framework string) (map[string]core.ClientBuildResult, error) {
			name := entryNames[0]
			return map[string]core.ClientBuildResult{
				name: {Script: "/dist/" + name + ".js"},
			}, nil
		},
	}
	service := NewBuildService(renderer, nil, &mockCLIOutput{}, nil)

	result := service.BuildProject(context.Background(), BuildInput{
		MainFile:    filepath.Join(tmpDir, "main.go"),
		OriginalCwd: tmpDir,
	})
	if result.Error != nil {
		t.Fatalf("BuildProject() error = %v", result.Error)
	}
	if !result.Success {
		t.Fatal("expected build success")
	}

	for _, stalePath := range []string{
		filepath.Join(tmpDir, ".bifrost", "dist", "stale.js"),
		filepath.Join(tmpDir, ".bifrost", "ssr", "stale.js"),
		filepath.Join(tmpDir, ".bifrost", "entries", "stale.tsx"),
		filepath.Join(tmpDir, ".bifrost", "pages", "stale.html"),
		filepath.Join(tmpDir, ".bifrost", "runtime", "stale-bin"),
		filepath.Join(tmpDir, ".bifrost", "public", "stale.txt"),
	} {
		if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
			t.Fatalf("expected stale artifact removed: %s", stalePath)
		}
	}

	if _, err := os.Stat(filepath.Join(tmpDir, ".bifrost", ".gitkeep")); err != nil {
		t.Fatalf("expected .bifrost root preserved: %v", err)
	}
}

func TestExportStaticPages_UsesRouteSpecificCriticalCSS(t *testing.T) {
	tmpDir := t.TempDir()
	distDir := filepath.Join(tmpDir, "dist")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "blog.css"), []byte(".hero{color:red}.cta{color:blue}"), 0o644); err != nil {
		t.Fatalf("write css: %v", err)
	}

	renderer := &fakeRenderer{
		renderFn: func(componentPath string, props any) (core.RenderedPage, error) {
			m, _ := props.(map[string]any); switch m["kind"] {
			case "hero":
				return core.RenderedPage{Body: `<div class="hero">Hero</div>`}, nil
			case "cta":
				return core.RenderedPage{Body: `<button class="cta">CTA</button>`}, nil
			default:
				return core.RenderedPage{Body: `<div>default</div>`}, nil
			}
		},
	}

	routes := []core.Route{
		core.Page("/blog/{slug}", "./pages/blog.tsx", core.WithStaticData(func(context.Context) ([]core.StaticPathData, error) {
			return []core.StaticPathData{
				{Path: "/blog/hero", Props: map[string]any{"kind": "hero"}},
				{Path: "/blog/cta", Props: map[string]any{"kind": "cta"}},
			}, nil
		})),
	}

	entryName := core.EntryNameForPath("./pages/blog.tsx")
	manifest := &core.Manifest{
		Entries: map[string]core.ManifestEntry{
			entryName: {
				Script:      "/dist/blog.js",
				CriticalCSS: ".hero{color:red}",
				CSS:         "/dist/blog.css",
				Mode:        "static",
			},
		},
	}

	err := ExportStaticPages(ExportStaticPagesInput{
		OutputDir: tmpDir,
		Routes:    routes,
		Manifest:  manifest,
		AppConfig: &core.Config{DefaultHTMLLang: "en"},
		SSBundlePath: func(string) string {
			return "/ssr/blog-ssr.js"
		},
		Renderer: renderer,
	})
	if err != nil {
		t.Fatalf("ExportStaticPages() error = %v", err)
	}

	heroHTML, err := os.ReadFile(filepath.Join(tmpDir, "pages", "routes", "blog", "hero", "index.html"))
	if err != nil {
		t.Fatalf("read hero html: %v", err)
	}
	ctaHTML, err := os.ReadFile(filepath.Join(tmpDir, "pages", "routes", "blog", "cta", "index.html"))
	if err != nil {
		t.Fatalf("read cta html: %v", err)
	}

	heroDoc := string(heroHTML)
	ctaDoc := string(ctaHTML)
	if !strings.Contains(heroDoc, ".hero{color:red}") {
		t.Fatalf("expected hero critical CSS in hero route: %s", heroDoc)
	}
	if strings.Contains(heroDoc, ".cta{color:blue}") {
		t.Fatalf("did not expect cta critical CSS in hero route: %s", heroDoc)
	}
	if !strings.Contains(ctaDoc, ".cta{color:blue}") {
		t.Fatalf("expected cta critical CSS in cta route: %s", ctaDoc)
	}
	if strings.Contains(ctaDoc, ".hero{color:red}") {
		t.Fatalf("did not expect hero critical CSS in cta route: %s", ctaDoc)
	}
}

func TestBuildProjectBatchesSSRBundles(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "main.go"), `package main
func main() {
	_ = Page("/", "./pages/home.tsx")
	_ = Page("/about", "./pages/about.tsx")
}`)
	writeTestFile(t, filepath.Join(tmpDir, "pages", "home.tsx"), "<title>Home</title>")
	writeTestFile(t, filepath.Join(tmpDir, "pages", "about.tsx"), "<title>About</title>")

	renderer := &fakeRenderer{
		buildFn: func(entrypoints []string, outdir string, entryNames []string, framework string) (map[string]core.ClientBuildResult, error) {
			result := make(map[string]core.ClientBuildResult, len(entryNames))
			for _, name := range entryNames {
				result[name] = core.ClientBuildResult{Script: "/dist/" + name + ".js"}
			}
			return result, nil
		},
		buildSSRFn: func(entrypoints []string, outdir string, framework string) error {
			for _, entryPath := range entrypoints {
				name := strings.TrimSuffix(filepath.Base(entryPath), filepath.Ext(entryPath))
				writeTestFile(t, filepath.Join(outdir, name+".js"), "// ssr")
			}
			return nil
		},
	}
	service := NewBuildService(renderer, nil, &mockCLIOutput{}, nil)
	service.compileRuntimeFn = func(bifrostDir string) error { return nil }

	result := service.BuildProject(context.Background(), BuildInput{
		MainFile:    filepath.Join(tmpDir, "main.go"),
		OriginalCwd: tmpDir,
	})
	if result.Error != nil {
		t.Fatalf("BuildProject() error = %v", result.Error)
	}
	if !result.Success {
		t.Fatal("expected build success")
	}
	if renderer.buildSSRCalls != 1 {
		t.Fatalf("expected one batched SSR build, got %d", renderer.buildSSRCalls)
	}
	if len(renderer.buildSSRBatchSizes) != 1 || renderer.buildSSRBatchSizes[0] != 2 {
		t.Fatalf("expected one SSR batch of size 2, got %v", renderer.buildSSRBatchSizes)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, ".bifrost", "ssr", "pages-home-entry-ssr.js")); err != nil {
		t.Fatalf("expected home SSR bundle: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".bifrost", "ssr", "pages-about-entry-ssr.js")); err != nil {
		t.Fatalf("expected about SSR bundle: %v", err)
	}
}

func TestBuildProjectFallsBackToPerPageSSRBundles(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "main.go"), `package main
func main() {
	_ = Page("/", "./pages/home.tsx")
	_ = Page("/about", "./pages/about.tsx")
}`)
	writeTestFile(t, filepath.Join(tmpDir, "pages", "home.tsx"), "<title>Home</title>")
	writeTestFile(t, filepath.Join(tmpDir, "pages", "about.tsx"), "<title>About</title>")

	renderer := &fakeRenderer{
		buildFn: func(entrypoints []string, outdir string, entryNames []string, framework string) (map[string]core.ClientBuildResult, error) {
			result := make(map[string]core.ClientBuildResult, len(entryNames))
			for _, name := range entryNames {
				result[name] = core.ClientBuildResult{Script: "/dist/" + name + ".js"}
			}
			return result, nil
		},
		buildSSRFn: func(entrypoints []string, outdir string, framework string) error {
			if len(entrypoints) > 1 {
				return errors.New("batch failed")
			}
			name := strings.TrimSuffix(filepath.Base(entrypoints[0]), filepath.Ext(entrypoints[0]))
			writeTestFile(t, filepath.Join(outdir, name+".js"), "// ssr")
			return nil
		},
	}
	service := NewBuildService(renderer, nil, &mockCLIOutput{}, nil)
	service.compileRuntimeFn = func(bifrostDir string) error { return nil }

	result := service.BuildProject(context.Background(), BuildInput{
		MainFile:    filepath.Join(tmpDir, "main.go"),
		OriginalCwd: tmpDir,
	})
	if result.Error != nil {
		t.Fatalf("BuildProject() error = %v", result.Error)
	}
	if !result.Success {
		t.Fatal("expected build success")
	}
	if renderer.buildSSRCalls != 3 {
		t.Fatalf("expected one batch SSR build and two fallback builds, got %d", renderer.buildSSRCalls)
	}
	if got := renderer.buildSSRBatchSizes; len(got) != 3 || got[0] != 2 || got[1] != 1 || got[2] != 1 {
		t.Fatalf("unexpected SSR batch sizes: %v", got)
	}
}

func TestBuildProjectFailsWhenMultipleNestedSSRBundlesExist(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "main.go"), `package main
func main() {
	_ = Page("/", "./pages/home.tsx")
}`)
	writeTestFile(t, filepath.Join(tmpDir, "pages", "home.tsx"), "<title>Home</title>")

	renderer := &fakeRenderer{
		buildFn: func(entrypoints []string, outdir string, entryNames []string, framework string) (map[string]core.ClientBuildResult, error) {
			return map[string]core.ClientBuildResult{
				entryNames[0]: {Script: "/dist/" + entryNames[0] + ".js"},
			}, nil
		},
		buildSSRFn: func(entrypoints []string, outdir string, framework string) error {
			name := strings.TrimSuffix(filepath.Base(entrypoints[0]), filepath.Ext(entrypoints[0]))
			writeTestFile(t, filepath.Join(outdir, ".bifrost", "entries", name+".js"), "// misplaced ssr")
			writeTestFile(t, filepath.Join(outdir, "nested", name+".js"), "// misplaced ssr duplicate")
			return nil
		},
	}
	service := NewBuildService(renderer, nil, &mockCLIOutput{}, nil)
	service.compileRuntimeFn = func(bifrostDir string) error { return nil }

	result := service.BuildProject(context.Background(), BuildInput{
		MainFile:    filepath.Join(tmpDir, "main.go"),
		OriginalCwd: tmpDir,
	})
	if result.Error != nil {
		t.Fatalf("BuildProject() error = %v", result.Error)
	}
	if result.Success {
		t.Fatal("expected build failure when multiple nested SSR bundles exist")
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, ".bifrost", "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if strings.Contains(string(data), `"ssr":`) {
		t.Fatalf("did not expect SSR manifest entry when SSR bundle is missing: %s", string(data))
	}
}

func chdirForTest(t *testing.T, dir string) func() {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatal(err)
		}
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDeferredLoaderRunsConcurrentlyWithRender(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "pages", "home.tsx"), "export default function Page(){ return <div>Hello</div> }")

	var deferredStart time.Time
	var renderStart time.Time
	var mu sync.Mutex

	renderer := &fakeRenderer{
		buildSSRFn: func(entrypoints []string, outdir string, framework string) error {
			name := strings.TrimSuffix(filepath.Base(entrypoints[0]), filepath.Ext(entrypoints[0]))
			writeTestFile(t, filepath.Join(outdir, name+".js"), "// ssr")
			return nil
		},
		renderFn: func(componentPath string, props any) (core.RenderedPage, error) {
			mu.Lock()
			renderStart = time.Now()
			mu.Unlock()
			return core.RenderedPage{Head: "<title>Home</title>", Body: "<div>Hello</div>"}, nil
		},
	}
	service := NewPageService(renderer, nil, nil)

	restore := chdirForTest(t, tmpDir)
	defer restore()

	input := ServePageInput{
		Config: core.PageConfig{
			ComponentPath: "./pages/home.tsx",
			Mode:          core.ModeSSR,
			PropsLoader: func(*http.Request) (any, error) {
				return map[string]any{"locale": "en"}, nil
			},
			DeferredPropsLoader: func(*http.Request) (any, error) {
				mu.Lock()
				deferredStart = time.Now()
				mu.Unlock()
				time.Sleep(50 * time.Millisecond)
				return map[string]any{"user": "alice"}, nil
			},
		},
		DefaultHTMLLang: "en",
		IsDev:           true,
		EntryName:       core.EntryNameForPath("./pages/home.tsx"),
		RequestPath:     "/",
		Request:         httptest.NewRequest(http.MethodGet, "/", nil),
	}

	output := service.ServePage(context.Background(), input)
	if output.Error != nil {
		t.Fatalf("ServePage() error = %v", output.Error)
	}
	if output.HTML == "" {
		t.Fatal("expected HTML output")
	}

	if !strings.Contains(output.HTML, `"user":"alice"`) {
		t.Fatalf("expected deferred props in __BIFROST_PROPS__, got %q", output.HTML)
	}
	if !strings.Contains(output.HTML, `"locale":"en"`) {
		t.Fatalf("expected sync props in __BIFROST_PROPS__, got %q", output.HTML)
	}

	mu.Lock()
	started := deferredStart
	rendStart := renderStart
	mu.Unlock()
	if started.IsZero() || rendStart.IsZero() {
		t.Fatal("expected both deferred loader and render to run")
	}
}

func TestDeferredLoaderErrorFallsBackToSyncProps(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "pages", "home.tsx"), "export default function Page(){ return <div>Hello</div> }")

	renderer := &fakeRenderer{
		buildSSRFn: func(entrypoints []string, outdir string, framework string) error {
			name := strings.TrimSuffix(filepath.Base(entrypoints[0]), filepath.Ext(entrypoints[0]))
			writeTestFile(t, filepath.Join(outdir, name+".js"), "// ssr")
			return nil
		},
		renderFn: func(componentPath string, props any) (core.RenderedPage, error) {
			return core.RenderedPage{Head: "<title>Home</title>", Body: "<div>Hello</div>"}, nil
		},
	}
	service := NewPageService(renderer, nil, nil)

	restore := chdirForTest(t, tmpDir)
	defer restore()

	input := ServePageInput{
		Config: core.PageConfig{
			ComponentPath: "./pages/home.tsx",
			Mode:          core.ModeSSR,
			PropsLoader: func(*http.Request) (any, error) {
				return map[string]any{"locale": "en"}, nil
			},
			DeferredPropsLoader: func(*http.Request) (any, error) {
				return nil, errors.New("db connection failed")
			},
		},
		DefaultHTMLLang: "en",
		IsDev:           true,
		EntryName:       core.EntryNameForPath("./pages/home.tsx"),
		RequestPath:     "/",
		Request:         httptest.NewRequest(http.MethodGet, "/", nil),
	}

	output := service.ServePage(context.Background(), input)
	if output.Error != nil {
		t.Fatalf("ServePage() error = %v", output.Error)
	}

	if strings.Contains(output.HTML, `"user"`) {
		t.Fatalf("did not expect deferred props when loader errors, got %q", output.HTML)
	}
	if !strings.Contains(output.HTML, `"locale":"en"`) {
		t.Fatalf("expected sync props in __BIFROST_PROPS__, got %q", output.HTML)
	}
}

func TestDeferredLoaderWithoutSyncLoader(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "pages", "home.tsx"), "export default function Page(){ return <div>Hello</div> }")

	renderer := &fakeRenderer{
		buildSSRFn: func(entrypoints []string, outdir string, framework string) error {
			name := strings.TrimSuffix(filepath.Base(entrypoints[0]), filepath.Ext(entrypoints[0]))
			writeTestFile(t, filepath.Join(outdir, name+".js"), "// ssr")
			return nil
		},
		renderFn: func(componentPath string, props any) (core.RenderedPage, error) {
			return core.RenderedPage{Head: "<title>Home</title>", Body: "<div>Hello</div>"}, nil
		},
	}
	service := NewPageService(renderer, nil, nil)

	restore := chdirForTest(t, tmpDir)
	defer restore()

	input := ServePageInput{
		Config: core.PageConfig{
			ComponentPath: "./pages/home.tsx",
			Mode:          core.ModeSSR,
			DeferredPropsLoader: func(*http.Request) (any, error) {
				return map[string]any{"user": "bob"}, nil
			},
		},
		DefaultHTMLLang: "en",
		IsDev:           true,
		EntryName:       core.EntryNameForPath("./pages/home.tsx"),
		RequestPath:     "/",
		Request:         httptest.NewRequest(http.MethodGet, "/", nil),
	}

	output := service.ServePage(context.Background(), input)
	if output.Error != nil {
		t.Fatalf("ServePage() error = %v", output.Error)
	}

	if !strings.Contains(output.HTML, `"user":"bob"`) {
		t.Fatalf("expected deferred props in __BIFROST_PROPS__, got %q", output.HTML)
	}
}

func TestBuildProjectGroupsByFramework(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "main.go"), `package main
func main() {
	_ = Page("/", "./pages/home.tsx")
	_ = Page("/svelte", "./pages/hello.svelte")
}`)
	writeTestFile(t, filepath.Join(tmpDir, "pages", "home.tsx"), "export default function Page(){ return <div>Hello</div> }")
	writeTestFile(t, filepath.Join(tmpDir, "pages", "hello.svelte"), "<script>let { name } = $props();</script><h1>Hello {name}</h1>")

	frameworks := make(map[string]int)
	renderer := &fakeRenderer{
		buildFn: func(entrypoints []string, outdir string, entryNames []string, framework string) (map[string]core.ClientBuildResult, error) {
			frameworks[framework]++
			result := make(map[string]core.ClientBuildResult, len(entryNames))
			for _, name := range entryNames {
				result[name] = core.ClientBuildResult{Script: "/dist/" + name + ".js"}
			}
			return result, nil
		},
		buildSSRFn: func(entrypoints []string, outdir string, framework string) error {
			frameworks[framework+"-ssr"]++
			for _, entryPath := range entrypoints {
				name := strings.TrimSuffix(filepath.Base(entryPath), filepath.Ext(entryPath))
				writeTestFile(t, filepath.Join(outdir, name+".js"), "// ssr")
			}
			return nil
		},
	}
	service := NewBuildService(renderer, nil, &mockCLIOutput{}, nil)
	service.compileRuntimeFn = func(bifrostDir string) error { return nil }

	result := service.BuildProject(context.Background(), BuildInput{
		MainFile:    filepath.Join(tmpDir, "main.go"),
		OriginalCwd: tmpDir,
	})
	if result.Error != nil {
		t.Fatalf("BuildProject() error = %v", result.Error)
	}
	if !result.Success {
		t.Fatal("expected build success")
	}

	if frameworks["react"] != 1 {
		t.Fatalf("expected one react client build group, got %v", frameworks)
	}
	if frameworks["svelte"] != 1 {
		t.Fatalf("expected one svelte client build group, got %v", frameworks)
	}
	if frameworks["react-ssr"] != 1 {
		t.Fatalf("expected one react ssr build group, got %v", frameworks)
	}
	if frameworks["svelte-ssr"] != 1 {
		t.Fatalf("expected one svelte ssr build group, got %v", frameworks)
	}
}

func TestDevSSRUsesFrameworkAdapterFromComponentPath(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "pages", "hello.svelte"), "<script>let { name } = $props();</script><h1>Hello {name}</h1>")

	var capturedEntrypoints []string
	var capturedFrameworks []string
	renderer := &fakeRenderer{
		buildFn: func(entrypoints []string, outdir string, entryNames []string, framework string) (map[string]core.ClientBuildResult, error) {
			capturedEntrypoints = append(capturedEntrypoints, entrypoints...)
			capturedFrameworks = append(capturedFrameworks, framework)
			return map[string]core.ClientBuildResult{
				entryNames[0]: {Script: "/dist/" + entryNames[0] + ".js"},
			}, nil
		},
		buildSSRFn: func(entrypoints []string, outdir string, framework string) error {
			capturedEntrypoints = append(capturedEntrypoints, entrypoints...)
			capturedFrameworks = append(capturedFrameworks, framework+"-ssr")
			name := strings.TrimSuffix(filepath.Base(entrypoints[0]), filepath.Ext(entrypoints[0]))
			writeTestFile(t, filepath.Join(outdir, name+".js"), "// ssr")
			return nil
		},
		renderFn: func(componentPath string, props any) (core.RenderedPage, error) {
			return core.RenderedPage{Body: "<h1>Hello Svelte</h1>"}, nil
		},
	}

	// PageService is constructed with the default React adapter, matching what
	// bifrost.New() creates for a mixed-framework app.
	service := NewPageService(renderer, nil, nil)

	restore := chdirForTest(t, tmpDir)
	defer restore()

	input := ServePageInput{
		Config: core.PageConfig{
			ComponentPath: "./pages/hello.svelte",
			Mode:          core.ModeSSR,
		},
		DefaultHTMLLang: "en",
		IsDev:           true,
		EntryName:       core.EntryNameForPath("./pages/hello.svelte"),
		RequestPath:     "/svelte",
		Request:         httptest.NewRequest(http.MethodGet, "/svelte", nil),
	}

	output := service.ServePage(context.Background(), input)
	if output.Error != nil {
		t.Fatalf("ServePage() error = %v", output.Error)
	}

	var ssrEntry string
	for _, ep := range capturedEntrypoints {
		if strings.HasSuffix(ep, "-ssr.ts") || strings.HasSuffix(ep, "-ssr.tsx") {
			ssrEntry = ep
			break
		}
	}
	if ssrEntry == "" {
		t.Fatalf("expected an SSR entrypoint to be generated, got %v", capturedEntrypoints)
	}

	content, err := os.ReadFile(ssrEntry)
	if err != nil {
		t.Fatalf("read ssr entry: %v", err)
	}

	if !strings.HasSuffix(ssrEntry, ".ts") {
		t.Fatalf("expected Svelte SSR entry to use .ts extension, got %q", ssrEntry)
	}
	if !strings.Contains(string(content), "svelte/server") {
		t.Fatalf("expected Svelte SSR entry to import from svelte/server, got:\n%s", string(content))
	}
}
