package usecase

import (
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/core"
)

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

func (s *BuildService) scanPages(mainFile string) ([]core.PageConfig, string, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, mainFile, nil, parser.ParseComments)
	if err != nil {
		return nil, "", err
	}

	defaultHTMLLang := scanDefaultHTMLLang(node)

	var configs []core.PageConfig
	seen := make(map[string]bool)

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
		case *ast.Ident:
			funcName = fn.Name
		default:
			return true
		}

		if funcName != "Page" {
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

	return configs, defaultHTMLLang, nil
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
