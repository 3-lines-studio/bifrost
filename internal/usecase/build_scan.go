package usecase

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
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

func parsePageBuildOptions(args []ast.Expr) (htmlLang string, htmlClass string, err error) {
	for _, arg := range args {
		call, ok := arg.(*ast.CallExpr)
		if !ok {
			return "", "", fmt.Errorf("page options must be direct bifrost option calls")
		}
		name := callExprSimpleName(call)
		switch name {
		case "WithLoader", "WithClient", "WithStatic", "WithStaticData":
			continue
		case "WithHTMLLang", "WithHTMLClass":
			if len(call.Args) != 1 {
				return "", "", fmt.Errorf("%s requires one string literal", name)
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return "", "", fmt.Errorf("%s requires a string literal during build", name)
			}
			value, unquoteErr := strconv.Unquote(lit.Value)
			if unquoteErr != nil {
				return "", "", fmt.Errorf("invalid %s value: %w", name, unquoteErr)
			}
			if name == "WithHTMLLang" {
				htmlLang = value
			} else {
				htmlClass = value
			}
		default:
			return "", "", fmt.Errorf("unsupported page option %q during build", name)
		}
	}
	return htmlLang, htmlClass, nil
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

func (s *BuildService) scanFileForPages(fset *token.FileSet, node *ast.File) ([]core.PageConfig, map[string]bool, error) {
	var configs []core.PageConfig
	seen := make(map[string]bool)
	modes := make(map[string]core.PageMode)
	importMap := buildImportMap(node)
	var conflictErr error

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
			conflictErr = fmt.Errorf("page component path must be a string literal at %s", fset.Position(callExpr.Pos()))
			return false
		}

		path, err := strconv.Unquote(lit.Value)
		if err != nil {
			conflictErr = fmt.Errorf("invalid Page component path at %s: %w", fset.Position(lit.Pos()), err)
			return false
		}

		mode, err := s.detectPageMode(callExpr.Args[argIndex:])
		if err != nil {
			conflictErr = fmt.Errorf("page %q: %w", path, err)
			return false
		}

		var optArgs []ast.Expr
		if len(callExpr.Args) > 2 {
			optArgs = callExpr.Args[2:]
		}
		htmlLang, htmlClass, err := parsePageBuildOptions(optArgs)
		if err != nil {
			conflictErr = fmt.Errorf("page %q: %w", path, err)
			return false
		}

		if previous, ok := modes[path]; ok {
			if previous != mode {
				conflictErr = fmt.Errorf(
					"component %q cannot use both %s and %s modes",
					path,
					previous.BuildLabel(),
					mode.BuildLabel(),
				)
				return false
			}
			return true
		}

		seen[path] = true
		modes[path] = mode
		configs = append(configs, core.PageConfig{
			ComponentPath:    path,
			Mode:             mode,
			HTMLLang:         htmlLang,
			HTMLClass:        htmlClass,
			StaticDataLoader: nil,
		})

		return true
	})

	return configs, seen, conflictErr
}

func (s *BuildService) scanPages(mainFile string) ([]core.PageConfig, string, error) {
	goFiles, err := goListMainModuleFiles(mainFile)
	if err != nil {
		return nil, "", fmt.Errorf("go list deps: %w", err)
	}

	fset := token.NewFileSet()
	var mergedConfigs []core.PageConfig
	globalModes := make(map[string]core.PageMode)
	var defaultHTMLLang string

	for _, goFile := range goFiles {
		node, err := parser.ParseFile(fset, goFile, nil, parser.ParseComments)
		if err != nil {
			return nil, "", fmt.Errorf("parse %s: %w", goFile, err)
		}

		if defaultHTMLLang == "" {
			defaultHTMLLang = scanDefaultHTMLLang(node)
		}

		configs, _, err := s.scanFileForPages(fset, node)
		if err != nil {
			return nil, "", fmt.Errorf("scan %s: %w", goFile, err)
		}
		for _, cfg := range configs {
			if previous, ok := globalModes[cfg.ComponentPath]; ok {
				if previous != cfg.Mode {
					return nil, "", fmt.Errorf(
						"component %q cannot use both %s and %s modes",
						cfg.ComponentPath,
						previous.BuildLabel(),
						cfg.Mode.BuildLabel(),
					)
				}
				continue
			}
			globalModes[cfg.ComponentPath] = cfg.Mode
			mergedConfigs = append(mergedConfigs, cfg)
		}
	}

	return mergedConfigs, defaultHTMLLang, nil
}

func (s *BuildService) detectPageMode(args []ast.Expr) (core.PageMode, error) {
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
		return core.ModeSSR, fmt.Errorf("conflicting page mode options")
	}

	if hasStaticPrerender {
		return core.ModeStaticPrerender, nil
	}

	if hasClientOnly {
		return core.ModeClientOnly, nil
	}

	return core.ModeSSR, nil
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
