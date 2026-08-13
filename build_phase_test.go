package bifrost

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

func TestDescribeBuildPhaseUsesDedicatedFD(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	t.Setenv(buildPhaseEnv, "describe")
	t.Setenv(buildFDEnv, strconv.Itoa(int(writer.Fd())))

	app, err := New(Config{SourceRoot: t.TempDir(), Routes: []Route{Client("/app", "pages/app.tsx")}})
	if err != nil {
		t.Fatal(err)
	}
	var result protocol.DescribeResult
	if err := json.NewDecoder(reader).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.SpecHash != app.specHash || len(result.Spec.Routes) != 1 || result.Limits.MaxPropsBytes != defaultMaxPropsBytes {
		t.Fatalf("bad describe result: %+v", result)
	}
}

func TestGenerateStaticValidatesOwnershipAndSorts(t *testing.T) {
	app, err := newApp(Config{
		SourceRoot: t.TempDir(),
		Routes: []Route{
			Static("/blog/{slug}", "pages/blog.tsx", func(context.Context) ([]StaticPage, error) {
				return []StaticPage{
					{Path: "/blog/z", Props: map[string]int{"n": 2}},
					{Path: "/blog/a", Props: map[string]int{"n": 1}, Document: Document{Lang: "ar", Class: "dark", Dir: "rtl"}},
				}, nil
			}),
			Static("/about", "pages/about.tsx", nil),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	pages, err := app.generateStatic(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{pages[0].Path, pages[1].Path, pages[2].Path}
	if got, want := strings.Join(paths, ","), "/about,/blog/a,/blog/z"; got != want {
		t.Fatalf("paths = %q, want %q", got, want)
	}
	if got := pages[1].Document; got != (protocol.DocumentAttributes{Lang: "ar", Class: "dark", Dir: "rtl"}) {
		t.Fatalf("document = %+v", got)
	}
}

func TestGenerateStaticRejectsInvalidDocumentAttributes(t *testing.T) {
	app, err := newApp(Config{SourceRoot: t.TempDir(), Routes: []Route{Static("/", "pages/home.tsx", func(context.Context) ([]StaticPage, error) {
		return []StaticPage{{Path: "/", Document: Document{Lang: "bad language"}}}, nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.generateStatic(context.Background()); err == nil || !strings.Contains(err.Error(), "document") {
		t.Fatalf("generate error = %v", err)
	}
}

func TestGenerateStaticPreservesExactTrailingSlash(t *testing.T) {
	app, err := newApp(Config{SourceRoot: t.TempDir(), Routes: []Route{Static("/docs/{$}", "pages/docs.tsx", nil)}})
	if err != nil {
		t.Fatal(err)
	}
	pages, err := app.generateStatic(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].Path != "/docs/" {
		t.Fatalf("pages = %+v", pages)
	}
}

func TestGenerateStaticRejectsPathClaimedByMoreSpecificRoute(t *testing.T) {
	app, err := newApp(Config{
		SourceRoot: t.TempDir(),
		Routes: []Route{
			Static("/blog/{slug}", "pages/blog.tsx", func(context.Context) ([]StaticPage, error) {
				return []StaticPage{{Path: "/blog/special"}}, nil
			}),
			Client("/blog/special", "pages/special.tsx"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.generateStatic(context.Background()); err == nil || !strings.Contains(err.Error(), "belongs to") {
		t.Fatalf("generate error = %v", err)
	}
}
