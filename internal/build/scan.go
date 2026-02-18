package build

import (
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"strconv"

	"github.com/3-lines-studio/bifrost/internal/types"
)

type PageInfo struct {
	Path                string
	Mode                types.PageMode
	HasStaticDataLoader bool
}

func scanPages(filename string) ([]PageInfo, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var configs []PageInfo
	seen := make(map[string]bool)

	ast.Inspect(node, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		var funcName string
		var argIndex int

		switch fn := callExpr.Fun.(type) {
		case *ast.SelectorExpr:
			funcName = fn.Sel.Name
			argIndex = 0
		case *ast.Ident:
			funcName = fn.Name
			argIndex = 1
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
		if !ok {
			slog.Warn("Page call with non-string argument", "position", fset.Position(callExpr.Pos()))
			return true
		}

		if lit.Kind != token.STRING {
			slog.Warn("Page call with non-string argument", "position", fset.Position(callExpr.Pos()))
			return true
		}

		path, err := strconv.Unquote(lit.Value)
		if err != nil {
			slog.Warn("failed to unquote string", "position", fset.Position(lit.Pos()), "error", err)
			return true
		}

		mode, hasLoader := detectPageOptions(callExpr.Args[argIndex:])

		if !seen[path] {
			seen[path] = true
			configs = append(configs, PageInfo{
				Path:                path,
				Mode:                mode,
				HasStaticDataLoader: hasLoader,
			})
		}

		return true
	})

	return configs, nil
}

func detectPageOptions(args []ast.Expr) (types.PageMode, bool) {
	hasClientOnly := false
	hasStaticPrerender := false
	hasStaticDataLoader := false

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
			hasStaticDataLoader = true
		}
	}

	if hasClientOnly && hasStaticPrerender {
		slog.Warn("Both WithClient and WithStatic detected, defaulting to SSR")
		return types.ModeSSR, hasStaticDataLoader
	}

	if hasStaticDataLoader && !hasStaticPrerender {
		slog.Warn("WithStaticData requires WithStatic")
	}

	if hasStaticPrerender {
		return types.ModeStaticPrerender, hasStaticDataLoader
	}

	if hasClientOnly {
		return types.ModeClientOnly, hasStaticDataLoader
	}

	return types.ModeSSR, hasStaticDataLoader
}
