package usecase

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/core"
)

type goListPackage struct {
	Dir     string
	GoFiles []string
	Module  *goListModule
}
type goListModule struct {
	Main bool
}

var (
	titleRegex         = regexp.MustCompile(`<title>([^}]+?)</title>`)
	titleTemplateRegex = regexp.MustCompile(`<title>\{` + "`" + `([^}]+?)` + "`" + `\}</title>`)
)

func callExprSimpleName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		if fn.Sel != nil {
			return fn.Sel.Name
		}
	case *ast.Ident:
		return fn.Name
	}
	return ""
}

func scanDefaultHTMLLang(f *ast.File) string {
	var lang string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if callExprSimpleName(call) != "WithDefaultHTMLLang" || len(call.Args) < 1 {
			return true
		}
		if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if u, err := strconv.Unquote(lit.Value); err == nil {
				lang = u
			}
		}
		return true
	})
	return lang
}

func parsePageBuildOptions(args []ast.Expr) (htmlLang string, htmlClass string) {
	for _, arg := range args {
		call, ok := arg.(*ast.CallExpr)
		if !ok {
			continue
		}
		switch callExprSimpleName(call) {
		case "WithHTMLLang":
			if len(call.Args) < 1 {
				continue
			}
			if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				htmlLang, _ = strconv.Unquote(lit.Value)
			}
		case "WithHTMLClass":
			if len(call.Args) < 1 {
				continue
			}
			if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				htmlClass, _ = strconv.Unquote(lit.Value)
			}
		}
	}
	return htmlLang, htmlClass
}

func goListMainModuleFiles(mainFile string) ([]string, error) {
	cmd := exec.Command("go", "list", "-deps", "-json", ".")
	cmd.Dir = filepath.Dir(mainFile)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("go list pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}

	decoder := json.NewDecoder(stdout)
	var goFiles []string
	for {
		var pkg goListPackage
		if err := decoder.Decode(&pkg); err != nil {
			if err == io.EOF {
				break
			}
			_ = cmd.Wait()
			return nil, fmt.Errorf("failed to parse go list output: %w", err)
		}
		if pkg.Module != nil && pkg.Module.Main {
			for _, f := range pkg.GoFiles {
				goFiles = append(goFiles, filepath.Join(pkg.Dir, f))
			}
		}
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("go list wait: %w", err)
	}
	return goFiles, nil
}

func buildImportMap(file *ast.File) map[string]string {
	importMap := make(map[string]string)
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if imp.Name != nil {
			importMap[imp.Name.Name] = path
		} else {
			alias := path[strings.LastIndex(path, "/")+1:]
			importMap[alias] = path
		}
	}
	return importMap
}

const bifrostImportPath = "github.com/3-lines-studio/bifrost"

func (s *BuildService) scanFileForPages(fset *token.FileSet, node *ast.File) ([]core.PageConfig, map[string]bool) {
	var configs []core.PageConfig
	seen := make(map[string]bool)
	importMap := buildImportMap(node)

	ast.Inspect(node, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		var funcName string
		argIndex := 1

		switch fn := callExpr.Fun.(type) {
		case *ast.SelectorExpr:
			funcName = fn.Sel.Name
			if funcName != "Page" {
				return true
			}
			xIdent, ok := fn.X.(*ast.Ident)
			if !ok || importMap[xIdent.Name] != bifrostImportPath {
				return true
			}
		case *ast.Ident:
			funcName = fn.Name
			if funcName != "Page" {
				return true
			}
			return true
		default:
			return true
		}

		if len(callExpr.Args) <= argIndex {
			return true
		}

		firstArg := callExpr.Args[argIndex]
		lit, ok := firstArg.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			slog.Warn("Page call with non-string component path", "position", fset.Position(callExpr.Pos()))
			return true
		}

		path, err := strconv.Unquote(lit.Value)
		if err != nil {
			slog.Warn("Failed to unquote string", "position", fset.Position(lit.Pos()), "error", err)
			return true
		}

		mode := s.detectPageMode(callExpr.Args[argIndex:])

		var optArgs []ast.Expr
		if len(callExpr.Args) > 2 {
			optArgs = callExpr.Args[2:]
		}
		htmlLang, htmlClass := parsePageBuildOptions(optArgs)

		if !seen[path] {
			seen[path] = true
			configs = append(configs, core.PageConfig{
				ComponentPath:    path,
				Mode:             mode,
				HTMLLang:         htmlLang,
				HTMLClass:        htmlClass,
				StaticDataLoader: nil,
			})
		}

		return true
	})

	return configs, seen
}

func (s *BuildService) scanPages(mainFile string) ([]core.PageConfig, string, error) {
	goFiles, err := goListMainModuleFiles(mainFile)
	if err != nil {
		return nil, "", fmt.Errorf("go list deps: %w", err)
	}

	fset := token.NewFileSet()
	var mergedConfigs []core.PageConfig
	globalSeen := make(map[string]bool)
	var defaultHTMLLang string

	for _, goFile := range goFiles {
		node, err := parser.ParseFile(fset, goFile, nil, parser.ParseComments)
		if err != nil {
			return nil, "", fmt.Errorf("parse %s: %w", goFile, err)
		}

		if defaultHTMLLang == "" {
			defaultHTMLLang = scanDefaultHTMLLang(node)
		}

		configs, fileSeen := s.scanFileForPages(fset, node)
		for _, cfg := range configs {
			if !globalSeen[cfg.ComponentPath] {
				globalSeen[cfg.ComponentPath] = true
				mergedConfigs = append(mergedConfigs, cfg)
			}
		}
		_ = fileSeen
	}

	return mergedConfigs, defaultHTMLLang, nil
}

func (s *BuildService) detectPageMode(args []ast.Expr) core.PageMode {
	hasClientOnly := false
	hasStaticPrerender := false

	for _, arg := range args {
		callExpr, ok := arg.(*ast.CallExpr)
		if !ok {
			continue
		}

		var funcName string
		switch fn := callExpr.Fun.(type) {
		case *ast.SelectorExpr:
			funcName = fn.Sel.Name
		case *ast.Ident:
			funcName = fn.Name
		}

		switch funcName {
		case "WithClient":
			hasClientOnly = true
		case "WithStatic":
			hasStaticPrerender = true
		case "WithStaticData":
			hasStaticPrerender = true
		}
	}

	if hasClientOnly && hasStaticPrerender {
		return core.ModeSSR
	}

	if hasStaticPrerender {
		return core.ModeStaticPrerender
	}

	if hasClientOnly {
		return core.ModeClientOnly
	}

	return core.ModeSSR
}

func (s *BuildService) extractTitleFromComponent(componentPath string) string {
	data, err := os.ReadFile(componentPath)
	if err != nil {
		return ""
	}
	content := string(data)

	matches := titleRegex.FindStringSubmatch(content)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	matches = titleTemplateRegex.FindStringSubmatch(content)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	return ""
}
