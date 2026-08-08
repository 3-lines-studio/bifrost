package usecase

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os/exec"
	"path/filepath"
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

func bifrostCallName(call *ast.CallExpr, importMap map[string]string) (string, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel == nil {
		return "", false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	if !ok || importMap[qualifier.Name] != bifrostImportPath {
		return "", false
	}
	return selector.Sel.Name, true
}

func scanDefaultHTMLLang(f *ast.File, importMap map[string]string) string {
	var lang string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name, ok := bifrostCallName(call, importMap)
		if !ok || name != "WithDefaultHTMLLang" || len(call.Args) != 1 {
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

func isNilExpr(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}

func parsePageBuildOptions(args []ast.Expr, importMap map[string]string) (mode core.PageMode, htmlLang string, htmlClass string, err error) {
	var hasLoader, hasPreLoader, hasClient, hasStatic, hasStaticData bool

	for _, arg := range args {
		call, ok := arg.(*ast.CallExpr)
		if !ok {
			return core.ModeSSR, "", "", fmt.Errorf("page options must be direct bifrost option calls")
		}
		name, ok := bifrostCallName(call, importMap)
		if !ok {
			return core.ModeSSR, "", "", fmt.Errorf("page options must be direct bifrost option calls")
		}

		switch name {
		case "WithLoader":
			if len(call.Args) != 1 || isNilExpr(call.Args[0]) {
				return core.ModeSSR, "", "", fmt.Errorf("WithLoader requires one non-nil loader")
			}
			hasLoader = true
		case "WithPreLoader":
			if len(call.Args) != 1 || isNilExpr(call.Args[0]) {
				return core.ModeSSR, "", "", fmt.Errorf("WithPreLoader requires one non-nil loader")
			}
			hasPreLoader = true
		case "WithClient":
			if len(call.Args) != 0 {
				return core.ModeSSR, "", "", fmt.Errorf("WithClient does not accept arguments")
			}
			hasClient = true
		case "WithStatic":
			if len(call.Args) != 0 {
				return core.ModeSSR, "", "", fmt.Errorf("WithStatic does not accept arguments")
			}
			hasStatic = true
		case "WithStaticData":
			if len(call.Args) != 1 || isNilExpr(call.Args[0]) {
				return core.ModeSSR, "", "", fmt.Errorf("WithStaticData requires one non-nil loader")
			}
			hasStaticData = true
		case "WithHTMLLang", "WithHTMLClass":
			if len(call.Args) != 1 {
				return core.ModeSSR, "", "", fmt.Errorf("%s requires one string literal", name)
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return core.ModeSSR, "", "", fmt.Errorf("%s requires a string literal during build", name)
			}
			value, unquoteErr := strconv.Unquote(lit.Value)
			if unquoteErr != nil {
				return core.ModeSSR, "", "", fmt.Errorf("invalid %s value: %w", name, unquoteErr)
			}
			if name == "WithHTMLLang" {
				htmlLang = value
			} else {
				htmlClass = value
			}
		default:
			return core.ModeSSR, "", "", fmt.Errorf("unsupported page option %q during build", name)
		}
	}

	if hasClient && (hasStatic || hasStaticData) {
		return core.ModeSSR, "", "", fmt.Errorf("conflicting page mode options")
	}
	if hasStatic && hasStaticData {
		return core.ModeSSR, "", "", fmt.Errorf("WithStatic and WithStaticData cannot be combined")
	}
	if (hasLoader || hasPreLoader) && (hasClient || hasStatic || hasStaticData) {
		return core.ModeSSR, "", "", fmt.Errorf("WithLoader is only valid in SSR mode")
	}
	if hasStatic || hasStaticData {
		return core.ModeStaticPrerender, htmlLang, htmlClass, nil
	}
	if hasClient {
		return core.ModeClientOnly, htmlLang, htmlClass, nil
	}
	return core.ModeSSR, htmlLang, htmlClass, nil
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
	configsByPath := make(map[string]core.PageConfig)
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

		var optArgs []ast.Expr
		if len(callExpr.Args) > 2 {
			optArgs = callExpr.Args[2:]
		}
		mode, htmlLang, htmlClass, err := parsePageBuildOptions(optArgs, importMap)
		if err != nil {
			conflictErr = fmt.Errorf("page %q: %w", path, err)
			return false
		}

		config := core.PageConfig{
			ComponentPath: path,
			Mode:          mode,
			HTMLLang:      htmlLang,
			HTMLClass:     htmlClass,
		}
		if previous, ok := configsByPath[path]; ok {
			if previous.Mode != mode {
				conflictErr = fmt.Errorf(
					"component %q cannot use both %s and %s modes",
					path,
					previous.Mode.BuildLabel(),
					mode.BuildLabel(),
				)
				return false
			}
			if mode == core.ModeClientOnly && (previous.HTMLLang != htmlLang || previous.HTMLClass != htmlClass) {
				conflictErr = fmt.Errorf("client-only component %q cannot use different HTML attributes across routes", path)
				return false
			}
			return true
		}

		seen[path] = true
		configsByPath[path] = config
		configs = append(configs, config)

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
	globalConfigs := make(map[string]core.PageConfig)
	var defaultHTMLLang string

	for _, goFile := range goFiles {
		node, err := parser.ParseFile(fset, goFile, nil, parser.ParseComments)
		if err != nil {
			return nil, "", fmt.Errorf("parse %s: %w", goFile, err)
		}

		if defaultHTMLLang == "" {
			defaultHTMLLang = scanDefaultHTMLLang(node, buildImportMap(node))
		}

		configs, _, err := s.scanFileForPages(fset, node)
		if err != nil {
			return nil, "", fmt.Errorf("scan %s: %w", goFile, err)
		}
		for _, cfg := range configs {
			if previous, ok := globalConfigs[cfg.ComponentPath]; ok {
				if previous.Mode != cfg.Mode {
					return nil, "", fmt.Errorf(
						"component %q cannot use both %s and %s modes",
						cfg.ComponentPath,
						previous.Mode.BuildLabel(),
						cfg.Mode.BuildLabel(),
					)
				}
				if cfg.Mode == core.ModeClientOnly && (previous.HTMLLang != cfg.HTMLLang || previous.HTMLClass != cfg.HTMLClass) {
					return nil, "", fmt.Errorf("client-only component %q cannot use different HTML attributes across routes", cfg.ComponentPath)
				}
				continue
			}
			globalConfigs[cfg.ComponentPath] = cfg
			mergedConfigs = append(mergedConfigs, cfg)
		}
	}

	return mergedConfigs, defaultHTMLLang, nil
}
