package usecase

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestBuildImportMap(t *testing.T) {
	src := `package main
import (
	b "github.com/3-lines-studio/bifrost"
	"fmt"
	_ "embed"
)
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	m := buildImportMap(file)
	if got, ok := m["b"]; !ok || got != "github.com/3-lines-studio/bifrost" {
		t.Fatalf("expected alias b mapped to bifrost, got %q %v", got, ok)
	}
	if got, ok := m["fmt"]; !ok || got != "fmt" {
		t.Fatalf("expected fmt mapped to fmt, got %q %v", got, ok)
	}
	if _, ok := m["embed"]; ok {
		t.Fatal("did not expect blank import in import map")
	}
}

func TestScanFileForPages_IgnoresNonBifrostPageCalls(t *testing.T) {
	src := `package main
import "example.com/other"
func main() {
	other.Page("/", "./pages/home.tsx")
	Page("/local", "./pages/local.tsx")
}
func Page(a, b string) {}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	svc := &BuildService{}
	configs, _, err := svc.scanFileForPages(fset, file)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 0 {
		t.Fatalf("expected no pages from non-bifrost Page calls, got %v", configs)
	}
}

func TestScanFileForPages_DetectsAliasedBifrostPage(t *testing.T) {
	src := `package main
import bf "github.com/3-lines-studio/bifrost"
func main() {
	_ = bf.Page("/", "./pages/home.tsx", bf.WithClient())
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	svc := &BuildService{}
	configs, _, err := svc.scanFileForPages(fset, file)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 page, got %d", len(configs))
	}
	if configs[0].ComponentPath != "./pages/home.tsx" {
		t.Fatalf("unexpected component path: %q", configs[0].ComponentPath)
	}
	if configs[0].Mode != core.ModeClientOnly {
		t.Fatalf("expected client-only mode, got %v", configs[0].Mode)
	}
}

func TestScanFileForPages_DetectsQualifiedBifrostPage(t *testing.T) {
	src := `package main
import "github.com/3-lines-studio/bifrost"
func main() {
	_ = bifrost.Page("/about", "./pages/about.tsx", bifrost.WithStatic())
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	svc := &BuildService{}
	configs, _, err := svc.scanFileForPages(fset, file)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 page, got %d", len(configs))
	}
	if configs[0].ComponentPath != "./pages/about.tsx" {
		t.Fatalf("unexpected component path: %q", configs[0].ComponentPath)
	}
	if configs[0].Mode != core.ModeStaticPrerender {
		t.Fatalf("expected static-prerender mode, got %v", configs[0].Mode)
	}
}

func TestScanFileForPages_RejectsNonLiteralComponentPath(t *testing.T) {
	src := `package main
import "github.com/3-lines-studio/bifrost"
const home = "./pages/home.tsx"
func main() { _ = bifrost.Page("/", home) }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = (&BuildService{}).scanFileForPages(fset, file)
	if err == nil {
		t.Fatal("expected non-literal component path to fail")
	}
}

func TestScanFileForPages_RejectsIndirectOptions(t *testing.T) {
	src := `package main
import "github.com/3-lines-studio/bifrost"
func main() {
	opts := []bifrost.PageOption{bifrost.WithClient()}
	_ = bifrost.Page("/", "./pages/home.tsx", opts...)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = (&BuildService{}).scanFileForPages(fset, file)
	if err == nil {
		t.Fatal("expected indirect page options to fail")
	}
}

func TestScanFileForPages_RejectsMixedModesForSharedComponent(t *testing.T) {
	src := `package main
import "github.com/3-lines-studio/bifrost"
func main() {
	_ = bifrost.Page("/ssr", "./pages/home.tsx")
	_ = bifrost.Page("/client", "./pages/home.tsx", bifrost.WithClient())
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = (&BuildService{}).scanFileForPages(fset, file)
	if err == nil {
		t.Fatal("expected mixed modes to fail")
	}
}

func TestScanFileForPages_RejectsConflictingModeOptions(t *testing.T) {
	src := `package main
import "github.com/3-lines-studio/bifrost"
func main() {
	_ = bifrost.Page("/", "./pages/home.tsx", bifrost.WithClient(), bifrost.WithStatic())
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = (&BuildService{}).scanFileForPages(fset, file)
	if err == nil {
		t.Fatal("expected conflicting mode options to fail")
	}
}

func TestScanFileForPagesRejectsInvalidOptionContracts(t *testing.T) {
	tests := []struct {
		name string
		opts string
	}{
		{name: "loader on client page", opts: `bifrost.WithClient(), bifrost.WithLoader(load)`},
		{name: "loader on static page", opts: `bifrost.WithStatic(), bifrost.WithLoader(load)`},
		{name: "nil props loader", opts: `bifrost.WithLoader(nil)`},
		{name: "nil static data loader", opts: `bifrost.WithStaticData(nil)`},
		{name: "redundant static options", opts: `bifrost.WithStatic(), bifrost.WithStaticData(staticData)`},
		{name: "non bifrost option", opts: `other.WithClient()`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := `package main
import (
    "context"
    "net/http"
    "github.com/3-lines-studio/bifrost"
    other "example.com/other"
)
func load(*http.Request) (any, error) { return nil, nil }
func staticData(context.Context) ([]bifrost.StaticPathData, error) { return nil, nil }
func main() { _ = bifrost.Page("/", "./pages/home.tsx", ` + tt.opts + `) }
`
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := (&BuildService{}).scanFileForPages(fset, file); err == nil {
				t.Fatal("expected invalid options to fail")
			}
		})
	}
}

func TestScanFileForPagesRejectsDifferentClientAttrsForSharedComponent(t *testing.T) {
	src := `package main
import "github.com/3-lines-studio/bifrost"
func main() {
    _ = bifrost.Page("/first", "./pages/shared.tsx", bifrost.WithClient(), bifrost.WithHTMLLang("en"))
    _ = bifrost.Page("/second", "./pages/shared.tsx", bifrost.WithClient(), bifrost.WithHTMLLang("fr"))
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := (&BuildService{}).scanFileForPages(fset, file); err == nil {
		t.Fatal("expected client HTML attribute conflict")
	}
}

func TestScanFileForPages_DeduplicatesPaths(t *testing.T) {
	src := `package main
import "github.com/3-lines-studio/bifrost"
func main() {
	_ = bifrost.Page("/", "./pages/home.tsx")
	_ = bifrost.Page("/again", "./pages/home.tsx")
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	svc := &BuildService{}
	configs, seen, err := svc.scanFileForPages(fset, file)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 unique page, got %d", len(configs))
	}
	if !seen["./pages/home.tsx"] {
		t.Fatal("expected path in seen map")
	}
}
