package usecase

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

type fakeRenderer struct {
	buildCalls           int
	buildSSRCalls        int
	buildSSRBatchSizes   []int
	individualBuildCalls int
	renderCalls          int
	streamCalls          int
	buildFn              func(entrypoints []string, outdir string, entryNames []string) (map[string]core.ClientBuildResult, error)
	buildSSRFn           func(entrypoints []string, outdir string) error
	renderFn             func(componentPath string, props map[string]any) (core.RenderedPage, error)
	streamFn             func(ctx context.Context, componentPath string, props map[string]any, w http.ResponseWriter, flush func(), onHead func(head string) error) error
}

func (f *fakeRenderer) Render(componentPath string, props map[string]any) (core.RenderedPage, error) {
	f.renderCalls++
	if f.renderFn != nil {
		return f.renderFn(componentPath, props)
	}
	return core.RenderedPage{}, nil
}

func (f *fakeRenderer) RenderChunked(ctx context.Context, componentPath string, props map[string]any, onHead func(head string) error, onBody func(body string) error) error {
	return nil
}

func (f *fakeRenderer) RenderBodyStream(ctx context.Context, componentPath string, props map[string]any, w io.Writer, flush func(), onHead func(head string) error) error {
	httpWriter, _ := w.(http.ResponseWriter)
	f.streamCalls++
	if f.streamFn != nil {
		return f.streamFn(ctx, componentPath, props, httpWriter, flush, onHead)
	}
	return nil
}

func (f *fakeRenderer) Build(entrypoints []string, outdir string, entryNames []string) (map[string]core.ClientBuildResult, error) {
	f.buildCalls++
	if len(entryNames) == 1 {
		f.individualBuildCalls++
	}
	if f.buildFn != nil {
		return f.buildFn(entrypoints, outdir, entryNames)
	}
	return map[string]core.ClientBuildResult{}, nil
}

func (f *fakeRenderer) BuildSSR(entrypoints []string, outdir string) error {
	f.buildSSRCalls++
	f.buildSSRBatchSizes = append(f.buildSSRBatchSizes, len(entrypoints))
	if f.buildSSRFn != nil {
		return f.buildSSRFn(entrypoints, outdir)
	}
	return nil
}

func TestPageServiceDevSSRBuildsThenStreams(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "pages", "home.tsx"), "export default function Page(){ return <div>Hello</div> }")

	renderer := &fakeRenderer{
		streamFn: func(ctx context.Context, componentPath string, props map[string]any, w http.ResponseWriter, flush func(), onHead func(head string) error) error {
			if componentPath == "" {
				t.Fatal("expected render path")
			}
			if err := onHead("<title>Home</title>"); err != nil {
				return err
			}
			_, err := w.Write([]byte("<div>Hello</div>"))
			return err
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
	if output.Stream == nil {
		t.Fatal("expected stream response")
	}
	if renderer.buildCalls != 1 || renderer.buildSSRCalls != 1 {
		t.Fatalf("expected one dev setup build, got Build=%d BuildSSR=%d", renderer.buildCalls, renderer.buildSSRCalls)
	}

	rec := httptest.NewRecorder()
	if err := output.Stream(rec); err != nil {
		t.Fatalf("stream error = %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<div>Hello</div>") {
		t.Fatalf("expected streamed body, got %q", body)
	}
	if !strings.Contains(body, "<title>Home</title>") {
		t.Fatalf("expected streamed head, got %q", body)
	}
}

func TestPageServiceStaticPrerenderReturnsNotFoundForMissingPath(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "pages", "blog.tsx"), "export default function Page(){ return <div>Blog</div> }")

	renderer := &fakeRenderer{}
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
		buildFn: func(entrypoints []string, outdir string, entryNames []string) (map[string]core.ClientBuildResult, error) {
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
		buildFn: func(entrypoints []string, outdir string, entryNames []string) (map[string]core.ClientBuildResult, error) {
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
		buildFn: func(entrypoints []string, outdir string, entryNames []string) (map[string]core.ClientBuildResult, error) {
			result := make(map[string]core.ClientBuildResult, len(entryNames))
			for _, name := range entryNames {
				result[name] = core.ClientBuildResult{Script: "/dist/" + name + ".js"}
			}
			return result, nil
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
	if renderer.buildSSRCalls != 1 {
		t.Fatalf("expected one batched SSR build, got %d", renderer.buildSSRCalls)
	}
	if len(renderer.buildSSRBatchSizes) != 1 || renderer.buildSSRBatchSizes[0] != 2 {
		t.Fatalf("expected one SSR batch of size 2, got %v", renderer.buildSSRBatchSizes)
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
		buildFn: func(entrypoints []string, outdir string, entryNames []string) (map[string]core.ClientBuildResult, error) {
			result := make(map[string]core.ClientBuildResult, len(entryNames))
			for _, name := range entryNames {
				result[name] = core.ClientBuildResult{Script: "/dist/" + name + ".js"}
			}
			return result, nil
		},
		buildSSRFn: func(entrypoints []string, outdir string) error {
			if len(entrypoints) > 1 {
				return errors.New("batch failed")
			}
			return nil
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
	if renderer.buildSSRCalls != 3 {
		t.Fatalf("expected one batch SSR build and two fallback builds, got %d", renderer.buildSSRCalls)
	}
	if got := renderer.buildSSRBatchSizes; len(got) != 3 || got[0] != 2 || got[1] != 1 || got[2] != 1 {
		t.Fatalf("unexpected SSR batch sizes: %v", got)
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
