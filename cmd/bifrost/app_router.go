package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/3-lines-studio/bifrost"
	"github.com/3-lines-studio/bifrost/internal/builder"
)

type routeParam struct {
	Name     string
	Value    string
	Segments bool
}

type metadataSource struct {
	View     string
	Generate bool
}

type viewWrapper struct {
	Path     string
	Template bool
}

type appRoute struct {
	Directory    string
	Pattern      string
	Params       []routeParam
	View         string
	ImportPath   string
	Alias        string
	PageGo       bool
	Native       bool
	ScreenOnly   bool
	Screen       bool
	ScreenArgs   int
	ViewArgs     int
	ViewName     string
	MetadataName string
	GoNotFound   bool
	Layouts      []layoutRef
	Error        layoutRef
	ErrorLayouts []layoutRef
	ErrorMeta    bool
	ErrorLinks   bool
	NotFoundAt   int
	HasNotFound  bool
	HasProps     bool
	HasMetadata  bool
	MetadataArgs int
	HasLinks     bool
	HasLoader    bool
	HasCache     bool
	HasSitemap   bool
	SitemapKind  sitemapReturn
	HasHead      bool
	ErrorViews   []string
	NotFoundView string
	LoadingView  string
	NotFoundPage bool
}

// layoutRef is one layout.go between the route root and a page, the nearest
// first: each wraps the view of the page it holds.
type layoutRef struct {
	Directory  string
	ImportPath string
	Alias      string
}

type goDir struct {
	Directory   string
	Pattern     string
	Params      []routeParam
	ImportPath  string
	Alias       string
	Middleware  bool
	Serve       bool
	HTTPMethods []string
}

func appRouterRoots(dir, packagePath string) (string, string, bool, error) {
	projectRoot := packagePath
	if !filepath.IsAbs(projectRoot) {
		projectRoot = filepath.Join(dir, projectRoot)
	}
	projectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", "", false, err
	}
	rootPage := fileExists(filepath.Join(projectRoot, "page.tsx"))
	appPage := fileExists(filepath.Join(projectRoot, "app", "page.tsx"))
	if rootPage && appPage {
		return "", "", false, errors.New("bifrost: both page.tsx and app/page.tsx define the route root")
	}
	if rootPage {
		return projectRoot, projectRoot, true, nil
	}
	if appPage {
		return projectRoot, filepath.Join(projectRoot, "app"), true, nil
	}
	if hasPage(filepath.Join(projectRoot, "app")) || hasGoFile(filepath.Join(projectRoot, "app")) || hasGoPage(filepath.Join(projectRoot, "app")) || hasMarker(filepath.Join(projectRoot, "app")) {
		return projectRoot, filepath.Join(projectRoot, "app"), true, nil
	}
	if hasPage(projectRoot) || hasGoPage(projectRoot) || hasMarker(projectRoot) {
		return projectRoot, projectRoot, true, nil
	}
	return projectRoot, "", false, nil
}

func appRouterHint(projectRoot string, err error) error {
	if hasPage(projectRoot) || hasMarker(projectRoot) || !hasGoFile(projectRoot) {
		return err
	}
	return fmt.Errorf("%w\nbifrost: %s has route.go but no page.tsx and no app directory; an App Router app needs one of them", err, filepath.ToSlash(projectRoot))
}

func walkTree(root string, match func(path string, entry os.DirEntry) bool) bool {
	found := false
	_ = filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() && entry.Name() == ".bifrost" {
			return filepath.SkipDir
		}
		if filePath != root && match(filePath, entry) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func hasPage(root string) bool {
	return walkTree(root, func(_ string, entry os.DirEntry) bool {
		return !entry.IsDir() && entry.Name() == "page.tsx"
	})
}

func hasGoPage(root string) bool {
	return walkTree(root, func(filePath string, entry os.DirEntry) bool {
		return !entry.IsDir() && entry.Name() == "page.go" && hasFunction(filePath, "View")
	})
}

func hasGoFile(root string) bool {
	return walkTree(root, func(_ string, entry os.DirEntry) bool {
		return !entry.IsDir() && isRouterGoFile(entry.Name())
	})
}

func hasMarker(root string) bool {
	return walkTree(root, func(_ string, entry os.DirEntry) bool {
		return entry.IsDir() && isMarkerName(entry.Name())
	})
}

func isRouterGoFile(name string) bool {
	return name == "route.go" || name == "middleware.go" || name == "server.go"
}

func isMarkerName(name string) bool {
	if strings.HasPrefix(name, "_") {
		return true
	}
	if strings.HasSuffix(name, "~") {
		return true
	}
	return strings.TrimRight(name, "_") != name
}

type appRouter struct {
	ProjectRoot string
	RouteRoot   string
	WorkDir     string
	Package     string
	Output      string
	Executable  string
	RouteParams map[string][]string
	Rows        []routeRow
	NativeOnly  bool
	NativePages bool
}

func (a *appRouter) routeRows() []routeRow {
	if a == nil {
		return nil
	}
	return a.Rows
}

func goRouteRows(routes []appRoute, goDirs []goDir) []routeRow {
	var rows []routeRow
	for _, route := range routes {
		if route.GoNotFound {
			rows = append(rows, routeRow{kind: "native", pattern: route.Pattern, source: path.Join(route.Directory, "not-found.go")})
		}
		if route.Native && !route.GoNotFound {
			rows = append(rows, routeRow{kind: "native", pattern: route.Pattern, source: path.Join(route.Directory, "page.go")})
		}
	}
	for _, directory := range goDirs {
		for _, method := range directory.HTTPMethods {
			rows = append(rows, routeRow{kind: "api", pattern: strings.ToUpper(method) + " " + directory.Pattern, source: path.Join(directory.Directory, "route.go")})
		}
		if directory.Middleware {
			rows = append(rows, routeRow{kind: "middleware", pattern: middlewareScope(directory.Pattern), source: path.Join(directory.Directory, "middleware.go")})
		}
	}
	return rows
}

func middlewareScope(pattern string) string {
	prefix := strings.TrimSuffix(pattern, "/{$}")
	if prefix == "" {
		return "/*"
	}
	return prefix + "/*"
}

func prepareAppRouter(ctx context.Context, projectRoot, routeRoot string, renderConcurrency, renderQueue int) (appRouter, error) {
	if renderConcurrency < 0 {
		return appRouter{}, errors.New("bifrost: render concurrency must not be negative")
	}
	routes, err := discoverRoutes(routeRoot)
	if err != nil {
		return appRouter{}, err
	}
	goDirs, err := discoverGoDirs(routeRoot)
	if err != nil {
		return appRouter{}, err
	}
	if len(routes) == 0 && len(goDirs) == 0 {
		return appRouter{}, fmt.Errorf("bifrost: no page.tsx, page.go with View, or route.go found under %s", routeRoot)
	}
	routes, err = appendNotFoundRoutes(routeRoot, routes)
	if err != nil {
		return appRouter{}, err
	}
	routes, err = appendGoNotFoundRoutes(routeRoot, routes)
	if err != nil {
		return appRouter{}, err
	}
	for index := range routes {
		if !routes[index].Native {
			continue
		}
		chain, err := layoutChain(routeRoot, routes[index].Directory)
		if err != nil {
			return appRouter{}, err
		}
		routes[index].Layouts = chain
		if routes[index].GoNotFound {
			continue
		}
		view, err := errorView(routeRoot, routes[index].Directory)
		if err != nil {
			return appRouter{}, err
		}
		routes[index].Error = view
		if view.Directory == "" {
			continue
		}
		errorLayouts, err := layoutChain(routeRoot, view.Directory)
		if err != nil {
			return appRouter{}, err
		}
		routes[index].ErrorLayouts = errorLayouts
		routes[index].ErrorMeta = packageDeclares(filepath.Join(routeRoot, filepath.FromSlash(view.Directory)), "ErrorMetadata")
		routes[index].ErrorLinks = packageDeclares(filepath.Join(routeRoot, filepath.FromSlash(view.Directory)), "Links")
	}
	module, err := resolveModule(ctx, routeRoot, routes, len(goDirs) > 0)
	if err != nil {
		return appRouter{}, err
	}
	modulePath, moduleDir := module.Path, module.Dir
	for index := range routes {
		for layoutIndex := range routes[index].Layouts {
			routes[index].Layouts[layoutIndex].ImportPath = packagePath(modulePath, routes[index].Layouts[layoutIndex].Directory)
		}
		if routes[index].Error.Directory != "" {
			routes[index].Error.ImportPath = packagePath(modulePath, routes[index].Error.Directory)
		}
		for layoutIndex := range routes[index].ErrorLayouts {
			routes[index].ErrorLayouts[layoutIndex].ImportPath = packagePath(modulePath, routes[index].ErrorLayouts[layoutIndex].Directory)
		}
	}
	generated := filepath.Join(projectRoot, ".bifrost", "app")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		return appRouter{}, err
	}
	if err := checkAppPackages(ctx, moduleDir, appPackagePaths(modulePath, routes, goDirs)); err != nil {
		return appRouter{}, err
	}
	for index := range routes {
		if !routes[index].PageGo {
			continue
		}
		if routes[index].ImportPath == "" {
			routes[index].ImportPath = modulePath
		} else {
			routes[index].ImportPath = modulePath + "/" + filepath.ToSlash(routes[index].ImportPath)
		}
		if routes[index].GoNotFound {
			continue
		}
		routes[index].HasLoader = hasSymbol(ctx, moduleDir, routes[index].ImportPath, "Load")
		routes[index].HasCache = hasSymbol(ctx, moduleDir, routes[index].ImportPath, "Cache")
	}
	for index := range goDirs {
		if goDirs[index].Directory == "." {
			goDirs[index].ImportPath = modulePath
		} else {
			goDirs[index].ImportPath = modulePath + "/" + filepath.ToSlash(goDirs[index].Directory)
		}
		goDirs[index].Middleware = hasSymbol(ctx, moduleDir, goDirs[index].ImportPath, "Middleware")
		goDirs[index].Serve = hasSymbol(ctx, moduleDir, goDirs[index].ImportPath, "Serve")
		if goDirs[index].Serve && goDirs[index].Directory != "." {
			return appRouter{}, fmt.Errorf("bifrost: server.go is only valid at the app root")
		}
		for _, method := range []string{"Get", "Post", "Put", "Patch", "Delete", "Head", "Options"} {
			if hasSymbol(ctx, moduleDir, goDirs[index].ImportPath, method) {
				goDirs[index].HTTPMethods = append(goDirs[index].HTTPMethods, method)
			}
		}
	}
	aliases := make(map[string]string)
	for index := range routes {
		if routes[index].PageGo {
			routes[index].Alias = importAlias(aliases, routes[index].ImportPath)
		}
		for layoutIndex := range routes[index].Layouts {
			layout := &routes[index].Layouts[layoutIndex]
			layout.Alias = importAlias(aliases, layout.ImportPath)
		}
		if routes[index].Error.ImportPath != "" {
			routes[index].Error.Alias = importAlias(aliases, routes[index].Error.ImportPath)
		}
		for layoutIndex := range routes[index].ErrorLayouts {
			layout := &routes[index].ErrorLayouts[layoutIndex]
			layout.Alias = importAlias(aliases, layout.ImportPath)
		}
	}
	for index := range goDirs {
		goDirs[index].Alias = importAlias(aliases, goDirs[index].ImportPath)
	}
	pagePatterns := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		pagePatterns[route.Pattern] = struct{}{}
	}
	for _, directory := range goDirs {
		if _, exists := pagePatterns[directory.Pattern]; !exists {
			continue
		}
		for _, method := range directory.HTTPMethods {
			if method == "Get" || method == "Head" {
				return appRouter{}, fmt.Errorf("bifrost: %s in %s/route.go conflicts with page.tsx", method, directory.Directory)
			}
		}
	}
	if err := writeViews(projectRoot, routeRoot, routes); err != nil {
		return appRouter{}, err
	}
	// A package that answers os.Args turns the app binary into a multi-command
	// one: a cron job or a migration runs `./app <command>` instead of shipping a
	// second binary.
	commandsImport := ""
	if hasSymbol(ctx, moduleDir, module.Root+"/commands", "Run") {
		commandsImport = module.Root + "/commands"
	}
	browserBuild := slices.ContainsFunc(routes, func(route appRoute) bool { return route.Native }) && packageDeclaresType(projectRoot, "app")
	if err := writeAppRouterMain(projectRoot, generated, routes, goDirs, renderConcurrency, renderQueue, commandsImport); err != nil {
		return appRouter{}, err
	}
	if err := clearWasmStatic(projectRoot); err != nil {
		return appRouter{}, err
	}
	if err := writeAppRoutes(projectRoot, routes); err != nil {
		return appRouter{}, err
	}
	if err := writeAppRouterModule(generated, moduleDir, slices.ContainsFunc(routes, func(route appRoute) bool { return route.Native })); err != nil {
		return appRouter{}, err
	}
	if err := os.MkdirAll(filepath.Join(generated, "build"), 0o755); err != nil {
		return appRouter{}, err
	}
	if err := os.WriteFile(filepath.Join(generated, "build", "embed.placeholder"), nil, 0o644); err != nil {
		return appRouter{}, err
	}
	command := exec.CommandContext(ctx, "go", "mod", "tidy")
	command.Dir = generated
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return appRouter{}, fmt.Errorf("bifrost: prepare generated module: %w", err)
	}
	nativeOnly := len(routes) > 0 && !slices.ContainsFunc(routes, func(route appRoute) bool { return !route.Native })
	return appRouter{ProjectRoot: projectRoot, RouteRoot: routeRoot, WorkDir: generated, Package: ".", Output: filepath.Join(generated, "build"), Executable: filepath.Join(projectRoot, ".bifrost", "bifrost-app"), RouteParams: routeParams(routes), Rows: goRouteRows(routes, goDirs), NativeOnly: nativeOnly, NativePages: browserBuild}, nil
}

func routeParams(routes []appRoute) map[string][]string {
	params := make(map[string][]string, len(routes))
	for _, route := range routes {
		if len(route.Params) == 0 {
			continue
		}
		names := make([]string, 0, len(route.Params))
		for _, param := range route.Params {
			names = append(names, param.Name)
		}
		params[route.Pattern] = names
	}
	return params
}

type routeFiles struct {
	pageTSX string
	pageGo  string
}

func discoverRoutes(root string) ([]appRoute, error) {
	files := make(map[string]*routeFiles)
	var directories []string
	err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if skip, err := skipDirectory(filePath, entry); err != nil {
			return err
		} else if skip {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		switch entry.Name() {
		case "page.tsx", "page.go":
			relative, err := filepath.Rel(root, filepath.Dir(filePath))
			if err != nil {
				return err
			}
			key := filepath.ToSlash(relative)
			record := files[key]
			if record == nil {
				record = &routeFiles{}
				files[key] = record
				directories = append(directories, key)
			}
			if entry.Name() == "page.tsx" {
				record.pageTSX = filePath
			} else {
				record.pageGo = filePath
			}
			return nil
		default:
			if !isRouteView(entry.Name()) || !hasHeadExport(filePath) {
				return nil
			}
			relative, err := filepath.Rel(root, filePath)
			if err != nil {
				return err
			}
			return fmt.Errorf("bifrost: %s exports Head; keep it in page.tsx", filepath.ToSlash(relative))
		}
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(directories)
	patterns := make(map[string]string)
	var routes []appRoute
	for _, directory := range directories {
		record := files[directory]
		viewArgs, hasView := declarationParams(record.pageGo, "View")
		screenArgs, hasScreen := declarationParams(record.pageGo, "Screen")
		if record.pageTSX != "" && (hasView || hasScreen) {
			return nil, fmt.Errorf("bifrost: %s has page.tsx and View or Screen in page.go; a page is React or Go, not both", directory)
		}
		if record.pageTSX == "" && !hasView && !hasScreen {
			return nil, fmt.Errorf("bifrost: %s/page.go has neither View nor Screen", directory)
		}
		native := record.pageTSX == ""
		pattern, params, err := routePattern(directory)
		if err != nil {
			return nil, fmt.Errorf("bifrost: route %s: %w", directory, err)
		}
		if previous := patterns[pattern]; previous != "" {
			return nil, fmt.Errorf("bifrost: duplicate route %q from %s and %s", pattern, previous, directory)
		}
		patterns[pattern] = directory
		route := appRoute{Directory: directory, Pattern: pattern, Params: params, Native: native}
		if record.pageTSX != "" {
			route.View = filepath.ToSlash(strings.TrimPrefix(record.pageTSX, root+string(filepath.Separator)))
			route.HasHead = hasHeadExport(record.pageTSX)
		}
		if record.pageGo != "" {
			route.PageGo = true
			route.ImportPath = directory
			if directory == "." {
				route.ImportPath = ""
			}
			if native {
				route.ScreenOnly = !hasView
				route.ViewArgs = viewArgs
				if route.ViewArgs > 1 {
					return nil, fmt.Errorf("bifrost: %s View takes the page props or nothing", directory)
				}
				route.HasProps = declaresType(record.pageGo, "Props")
				route.MetadataArgs, route.HasMetadata = declarationParams(record.pageGo, "Metadata")
				if route.MetadataArgs > 1 {
					return nil, fmt.Errorf("bifrost: %s Metadata takes the page props or nothing", directory)
				}
				if (route.ViewArgs > 0 || route.MetadataArgs > 0) && !route.HasProps {
					return nil, fmt.Errorf("bifrost: %s View or Metadata takes props but page.go has no Props type", directory)
				}
				route.HasLinks = sourceDeclares(record.pageGo, "Links")
				kind, declared, err := sitemapDeclaration(filepath.Dir(record.pageGo))
				if err != nil {
					return nil, err
				}
				route.SitemapKind = kind
				route.HasSitemap = declared
				route.ScreenArgs, route.Screen = screenArgs, hasScreen
				if route.Screen && route.ScreenArgs > 1 {
					return nil, fmt.Errorf("bifrost: %s Screen takes one dependency or none", directory)
				}
			}
		}
		routes = append(routes, route)
	}
	slices.SortFunc(routes, func(a, b appRoute) int { return strings.Compare(a.Pattern, b.Pattern) })
	return routes, nil
}

// packageDeclaresType reports whether a Go file of the directory's package
// declares the type.
// packageDeclares reports whether a Go file of the directory's package
// declares the name.
func packageDeclares(directory, name string) bool {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return false
	}
	found := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		if sourceDeclares(filepath.Join(directory, entry.Name()), name) {
			found = true
		}
	}
	return found
}

func packageDeclaresType(directory, name string) bool {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return false
	}
	found := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		if declaresType(filepath.Join(directory, entry.Name()), name) {
			found = true
		}
	}
	return found
}

func declaresType(filePath, name string) bool {
	file, err := parser.ParseFile(token.NewFileSet(), filePath, nil, parser.SkipObjectResolution)
	if err != nil {
		return false
	}
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range general.Specs {
			if typed, ok := spec.(*ast.TypeSpec); ok && typed.Name.Name == name {
				return true
			}
		}
	}
	return false
}

func hasFunction(filePath, name string) bool {
	_, ok := declarationParams(filePath, name)
	return ok
}

func declarationParams(filePath, name string) (int, bool) {
	file, err := parser.ParseFile(token.NewFileSet(), filePath, nil, parser.SkipObjectResolution)
	if err != nil {
		return 0, false
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil || function.Name.Name != name {
			continue
		}
		return function.Type.Params.NumFields(), true
	}
	return 0, false
}

// sitemapReturn is how a page declares the paths it adds to the sitemap.
type sitemapReturn uint8

const (
	sitemapNone sitemapReturn = iota
	sitemapPaths
	sitemapEntries
)

// sitemapDeclaration returns how the page package in directory declares
// Sitemap, and whether it declares it at all. Only a function of no receiver
// that returns []string or []bifrost.SitemapEntry is one: anything else fails
// the build where the page is read, not where the server is generated.
func sitemapDeclaration(directory string) (sitemapReturn, bool, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return sitemapNone, false, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		filePath := filepath.Join(directory, name)
		file, err := parser.ParseFile(token.NewFileSet(), filePath, nil, parser.SkipObjectResolution)
		if err != nil {
			return sitemapNone, false, err
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || function.Name.Name != "Sitemap" {
				continue
			}
			if function.Type.Params.NumFields() != 0 || function.Type.Results == nil || function.Type.Results.NumFields() != 1 {
				return sitemapNone, false, fmt.Errorf("bifrost: %s: Sitemap takes no argument and returns []string or []bifrost.SitemapEntry", filePath)
			}
			array, ok := function.Type.Results.List[0].Type.(*ast.ArrayType)
			if !ok {
				return sitemapNone, false, fmt.Errorf("bifrost: %s: Sitemap returns []string or []bifrost.SitemapEntry", filePath)
			}
			switch element := array.Elt.(type) {
			case *ast.Ident:
				if element.Name == "string" {
					return sitemapPaths, true, nil
				}
			case *ast.SelectorExpr:
				if element.Sel.Name == "SitemapEntry" {
					return sitemapEntries, true, nil
				}
			}
			return sitemapNone, false, fmt.Errorf("bifrost: %s: Sitemap returns []string or []bifrost.SitemapEntry", filePath)
		}
	}
	return sitemapNone, false, nil
}

func sourceDeclares(filePath, name string) bool {
	file, err := parser.ParseFile(token.NewFileSet(), filePath, nil, parser.SkipObjectResolution)
	if err != nil {
		return false
	}
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			if declaration.Recv == nil && declaration.Name.Name == name {
				return true
			}
		case *ast.GenDecl:
			for _, spec := range declaration.Specs {
				switch spec := spec.(type) {
				case *ast.ValueSpec:
					for _, identifier := range spec.Names {
						if identifier.Name == name {
							return true
						}
					}
				case *ast.TypeSpec:
					if spec.Name.Name == name {
						return true
					}
				}
			}
		}
	}
	return false
}

// appendGoNotFoundRoutes adds a Native not-found page for each not-found.go,
// under the same path of its directory that not-found.tsx uses.
func appendGoNotFoundRoutes(root string, routes []appRoute) ([]appRoute, error) {
	patterns := make(map[string]string, len(routes))
	for _, route := range routes {
		if route.NotFoundPage {
			patterns[route.Pattern] = route.Directory
		}
	}
	var directories []string
	err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if skip, err := skipDirectory(filePath, entry); err != nil {
			return err
		} else if skip {
			return filepath.SkipDir
		}
		if entry.IsDir() || entry.Name() != "not-found.go" {
			return nil
		}
		relative, err := filepath.Rel(root, filepath.Dir(filePath))
		if err != nil {
			return err
		}
		directory := filepath.ToSlash(relative)
		params, hasView := declarationParams(filePath, "NotFound")
		if !hasView || params != 0 {
			return fmt.Errorf("bifrost: %s/not-found.go needs NotFound() with no arguments", directory)
		}
		if metadataParams, hasMetadata := declarationParams(filePath, "NotFoundMetadata"); hasMetadata && metadataParams != 0 {
			return fmt.Errorf("bifrost: %s/not-found.go NotFoundMetadata takes no arguments", directory)
		}
		directories = append(directories, directory)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, directory := range directories {
		filePath := filepath.Join(root, filepath.FromSlash(directory), "not-found.go")
		base, _, err := routePattern(directory)
		if err != nil {
			return nil, err
		}
		pattern := strings.TrimSuffix(base, "/{$}") + "/{path...}"
		if previous, exists := patterns[pattern]; exists {
			return nil, fmt.Errorf("bifrost: %s and %s both define the not-found page for %s", previous, directory, pattern)
		}
		patterns[pattern] = directory
		importPath := directory
		if directory == "." {
			importPath = ""
		}
		_, hasMetadata := declarationParams(filePath, "NotFoundMetadata")
		routes = append(routes, appRoute{
			Directory:    directory,
			Pattern:      pattern,
			Native:       true,
			PageGo:       true,
			GoNotFound:   true,
			ImportPath:   importPath,
			ViewName:     "NotFound",
			MetadataName: "NotFoundMetadata",
			HasMetadata:  hasMetadata,
		})
	}
	slices.SortFunc(routes, func(a, b appRoute) int { return strings.Compare(a.Pattern, b.Pattern) })
	return routes, nil
}

// layoutChain returns the layout.go files holding a page, the nearest first.
func layoutChain(root, directory string) ([]layoutRef, error) {
	var chain []layoutRef
	for current := directory; ; {
		filePath := filepath.Join(root, filepath.FromSlash(current), "layout.go")
		if fileExists(filePath) {
			if params, ok := declarationParams(filePath, "Layout"); !ok || params != 1 {
				return nil, fmt.Errorf("bifrost: %s/layout.go needs Layout(page func(*ui.Context)) func(*ui.Context)", current)
			}
			chain = append(chain, layoutRef{Directory: current})
		}
		if current == "." {
			return chain, nil
		}
		parent := path.Dir(current)
		if parent == "/" || parent == "" {
			parent = "."
		}
		current = parent
	}
}

// errorView returns the nearest error.go holding a page, which handlers the
// page's failures, and checks what it exports.
func errorView(root, directory string) (layoutRef, error) {
	for current := directory; ; {
		filePath := filepath.Join(root, filepath.FromSlash(current), "error.go")
		if fileExists(filePath) {
			if params, ok := declarationParams(filePath, "Error"); !ok || params != 1 {
				return layoutRef{}, fmt.Errorf("bifrost: %s/error.go needs Error(err error) func(*ui.Context)", current)
			}
			if params, ok := declarationParams(filePath, "ErrorMetadata"); ok && params != 0 {
				return layoutRef{}, fmt.Errorf("bifrost: %s/error.go ErrorMetadata takes no arguments", current)
			}
			return layoutRef{Directory: current}, nil
		}
		if current == "." {
			return layoutRef{}, nil
		}
		parent := path.Dir(current)
		if parent == "/" || parent == "" {
			parent = "."
		}
		current = parent
	}
}

func packagePath(modulePath, directory string) string {
	if directory == "" || directory == "." {
		return modulePath
	}
	return modulePath + "/" + filepath.ToSlash(directory)
}

func appendNotFoundRoutes(root string, routes []appRoute) ([]appRoute, error) {
	err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if skip, err := skipDirectory(filePath, entry); err != nil {
			return err
		} else if skip {
			return filepath.SkipDir
		}
		if entry.IsDir() || entry.Name() != "not-found.tsx" {
			return nil
		}
		relative, err := filepath.Rel(root, filepath.Dir(filePath))
		if err != nil {
			return err
		}
		base, _, err := routePattern(relative)
		if err != nil {
			return err
		}
		pattern := strings.TrimSuffix(base, "/{$}") + "/{path...}"
		routes = append(routes, appRoute{Directory: filepath.ToSlash(relative), Pattern: pattern, View: filepath.ToSlash(strings.TrimPrefix(filePath, root+string(filepath.Separator))), NotFoundPage: true})
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(routes, func(a, b appRoute) int { return strings.Compare(a.Pattern, b.Pattern) })
	return routes, nil
}

func discoverGoDirs(root string) ([]goDir, error) {
	byDirectory := make(map[string]goDir)
	err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if skip, err := skipDirectory(filePath, entry); err != nil {
			return err
		} else if skip {
			return filepath.SkipDir
		}
		if entry.IsDir() || !isRouterGoFile(entry.Name()) {
			return nil
		}
		relative, err := filepath.Rel(root, filepath.Dir(filePath))
		if err != nil {
			return err
		}
		pattern, params, err := routePattern(relative)
		if err != nil {
			return fmt.Errorf("bifrost: route %s: %w", filepath.ToSlash(relative), err)
		}
		directory := filepath.ToSlash(relative)
		item := byDirectory[directory]
		item.Directory = directory
		item.Pattern = pattern
		item.Params = params
		byDirectory[directory] = item
		return nil
	})
	if err != nil {
		return nil, err
	}
	items := make([]goDir, 0, len(byDirectory))
	for _, item := range byDirectory {
		items = append(items, item)
	}
	slices.SortFunc(items, func(a, b goDir) int { return strings.Compare(a.Directory, b.Directory) })
	return items, nil
}

func writeViews(projectRoot, routeRoot string, routes []appRoute) error {
	views := filepath.Join(projectRoot, ".bifrost", "views")
	if err := os.RemoveAll(views); err != nil {
		return err
	}
	if err := os.MkdirAll(views, 0o755); err != nil {
		return err
	}
	needsMetadata := false
	for index := range routes {
		if routes[index].Native {
			continue
		}
		wrappers := inheritedWrappers(routeRoot, routes[index].Directory)
		layouts := make([]string, 0, len(wrappers))
		for _, wrapper := range wrappers {
			if !wrapper.Template {
				layouts = append(layouts, wrapper.Path)
			}
		}
		errors := inheritedFiles(routeRoot, routes[index].Directory, "error.tsx")
		notFound := inheritedFiles(routeRoot, routes[index].Directory, "not-found.tsx")
		loadings := inheritedFiles(routeRoot, routes[index].Directory, "loading.tsx")
		if routes[index].NotFoundPage {
			errors = nil
			notFound = nil
			loadings = nil
		}
		routes[index].ErrorViews = errors
		if len(notFound) > 0 {
			routes[index].NotFoundView = notFound[len(notFound)-1]
		}
		if len(loadings) > 0 {
			routes[index].LoadingView = loadings[len(loadings)-1]
		}
		var imports strings.Builder
		imports.WriteString("import { Fragment } from 'react';\n")
		imports.WriteString("import { RouteProvider } from 'virtual:bifrost/navigation';\n")
		pageView := filepath.Join(routeRoot, filepath.FromSlash(routes[index].View))
		metadata, err := metadataSources(append(slices.Clone(layouts), pageView))
		if err != nil {
			return err
		}
		if routes[index].HasHead && len(metadata) > 0 {
			return fmt.Errorf("bifrost: %s exports both Head and metadata; keep one", filepath.ToSlash(routes[index].View))
		}
		for metadataIndex, source := range metadata {
			name, local := "metadata", fmt.Sprintf("Metadata%d", metadataIndex)
			if source.Generate {
				name, local = "generateMetadata", fmt.Sprintf("GenerateMetadata%d", metadataIndex)
			}
			fmt.Fprintf(&imports, "import { %s as %s } from %s;\n", name, local, strconv.Quote(source.View))
		}
		if len(metadata) > 0 {
			needsMetadata = true
			imports.WriteString("import { Metadata as RouteMetadata } from './metadata.tsx';\n")
		}
		export := "Page"
		if routes[index].NotFoundPage {
			export = "NotFound"
		}
		if err := writeImport(&imports, filepath.Join(routeRoot, filepath.FromSlash(routes[index].View)), export, "RoutePage"); err != nil {
			return err
		}
		for wrapperIndex, wrapper := range wrappers {
			name := "Layout"
			if wrapper.Template {
				name = "Template"
			}
			if err := writeImport(&imports, wrapper.Path, name, fmt.Sprintf("%s%d", name, wrapperIndex)); err != nil {
				return err
			}
		}
		for errorIndex, errorView := range routes[index].ErrorViews {
			if err := writeImport(&imports, errorView, "Error", fmt.Sprintf("ErrorPage%d", errorIndex)); err != nil {
				return err
			}
		}
		if routes[index].NotFoundView != "" {
			if err := writeImport(&imports, routes[index].NotFoundView, "NotFound", "NotFound"); err != nil {
				return err
			}
		}
		if routes[index].LoadingView != "" {
			if err := writeImport(&imports, routes[index].LoadingView, "Loading", "RouteLoading"); err != nil {
				return err
			}
		}
		body := "<RoutePage {...props} />"
		if routes[index].NotFoundPage {
			body = "<RoutePage />"
		}
		if len(routes[index].ErrorViews) > 0 {
			imports.WriteString("import { refresh } from 'virtual:bifrost/navigation';\n")
		}
		for errorIndex := range routes[index].ErrorViews {
			body = fmt.Sprintf("props.__bifrostError && props.__bifrostErrorLevel === %d ? <ErrorPage%d error={new Error(String(props.__bifrostError))} reset={() => void refresh()} /> : %s", errorIndex, errorIndex, body)
		}
		if routes[index].NotFoundView != "" {
			body = "props.__bifrostNotFound ? <NotFound /> : " + body
		}
		wrap := func(content string) string {
			wrapped := "<Fragment key={pageKey}>{" + content + "}</Fragment>"
			for wrapperIndex := len(wrappers) - 1; wrapperIndex >= 0; wrapperIndex-- {
				name := "Layout"
				key := strconv.Quote(strings.TrimPrefix(filepath.ToSlash(wrappers[wrapperIndex].Path), filepath.ToSlash(routeRoot)+"/"))
				if wrappers[wrapperIndex].Template {
					name, key = "Template", "pageKey"
				}
				wrapped = fmt.Sprintf("<%s%d key={%s} params={props.params}>%s</%s%d>", name, wrapperIndex, key, wrapped, name, wrapperIndex)
			}
			return "<RouteProvider pathname={props.pathname} params={props.params} searchParams={props.searchParams}>" + wrapped + "</RouteProvider>"
		}
		head := ""
		switch {
		case metadataUsesProps(metadata):
			head = "export async function renderHead(props: Record<string, unknown>) {\n  return <RouteMetadata values={[" + metadataValues(metadata) + "]} />;\n}\n"
		case len(metadata) > 0:
			head = "export function Head() {\n  return <RouteMetadata values={[" + metadataValues(metadata) + "]} />;\n}\n"
		case !routes[index].NotFoundPage:
			head = "export * from " + strconv.Quote(pageView) + ";\n"
		}
		source := imports.String() + head + "export function renderPage(props: Record<string, unknown>, pageKey?: string) {\n  \"use no memo\";\n  return " + wrap(body) + ";\n}\nexport function Page(props: Record<string, unknown>) {\n  return renderPage(props);\n}\n"
		if routes[index].LoadingView != "" {
			source += "export function renderPending(props: Record<string, unknown>, pageKey?: string) {\n  \"use no memo\";\n  return " + wrap("<RouteLoading />") + ";\n}\n"
		}
		name := fmt.Sprintf("page-%d.tsx", index)
		if err := os.WriteFile(filepath.Join(views, name), []byte(source), 0o644); err != nil {
			return err
		}
		routes[index].View = ".bifrost/views/" + name
	}
	if needsMetadata {
		if err := os.WriteFile(filepath.Join(views, "metadata.tsx"), []byte(metadataModuleSource), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func metadataValues(sources []metadataSource) string {
	values := make([]string, 0, len(sources))
	for index, source := range sources {
		if source.Generate {
			values = append(values, fmt.Sprintf("await GenerateMetadata%d(props)", index))
			continue
		}
		values = append(values, fmt.Sprintf("Metadata%d", index))
	}
	return strings.Join(values, ", ")
}

func metadataUsesProps(sources []metadataSource) bool {
	return slices.ContainsFunc(sources, func(source metadataSource) bool { return source.Generate })
}

var headExportPattern = regexp.MustCompile(`(?m)^\s*export\s+(?:function|const|let|var)\s+Head\b`)

var pseudoVersionPattern = regexp.MustCompile(`[.-]\d{14}-[0-9a-f]{12}(\+dirty)?$`)

func developmentVersion(version string) bool {
	return !strings.HasPrefix(version, "v") || pseudoVersionPattern.MatchString(version)
}

var defaultExportPattern = regexp.MustCompile(`(?m)^\s*export\s+default\b`)

var metadataExportPattern = regexp.MustCompile(`(?m)^\s*export\s+(?:const|let|var)\s+metadata\b`)

var generateMetadataExportPattern = regexp.MustCompile(`(?m)^\s*export\s+(?:async\s+)?(?:function|const|let|var)\s+generateMetadata\b`)

const metadataModuleSource = `import type { ReactNode } from 'react';

type Dict = Record<string, unknown>;

const metadataKeys = ['title', 'description', 'keywords', 'alternates', 'robots', 'openGraph'];
const alternatesKeys = ['canonical'];
const robotsKeys = ['index', 'follow'];
const openGraphKeys = ['title', 'description', 'url', 'images'];

function record(value: unknown, where: string, keys: string[]): Dict {
	if (typeof value !== 'object' || value === null || Array.isArray(value)) throw new Error('bifrost: ' + where + ' must be an object');
	const result = value as Dict;
	for (const key of Object.keys(result)) {
		if (result[key] === undefined) continue;
		if (!keys.includes(key)) throw new Error('bifrost: unsupported ' + where + ' key "' + key + '"; supported keys: ' + keys.join(', '));
	}
	return result;
}

function text(value: unknown, where: string): string {
	if (typeof value !== 'string') throw new Error('bifrost: ' + where + ' must be a string');
	return value;
}

function flag(value: unknown, where: string): boolean {
	if (typeof value !== 'boolean') throw new Error('bifrost: ' + where + ' must be a boolean');
	return value;
}

function list(value: unknown, where: string): string[] {
	if (typeof value === 'string') return [value];
	if (Array.isArray(value) && value.every(item => typeof item === 'string')) return value as string[];
	throw new Error('bifrost: ' + where + ' must be a string or an array of strings');
}

function merge(values: unknown[]): Dict {
	const merged: Dict = {};
	for (const value of values) {
		if (value === undefined) continue;
		const source = record(value, 'metadata', metadataKeys);
		for (const key of Object.keys(source)) {
			if (source[key] !== undefined) merged[key] = source[key];
		}
	}
	return merged;
}

export function Metadata({ values }: { values: unknown[] }) {
	const data = merge(values);
	const tags: ReactNode[] = [];
	if (data.title !== undefined) tags.push(<title key="title">{text(data.title, 'metadata.title')}</title>);
	if (data.description !== undefined) tags.push(<meta key="description" name="description" content={text(data.description, 'metadata.description')} />);
	if (data.keywords !== undefined) tags.push(<meta key="keywords" name="keywords" content={list(data.keywords, 'metadata.keywords').join(', ')} />);
	if (data.alternates !== undefined) {
		const alternates = record(data.alternates, 'metadata.alternates', alternatesKeys);
		if (alternates.canonical !== undefined) tags.push(<link key="canonical" rel="canonical" href={text(alternates.canonical, 'metadata.alternates.canonical')} />);
	}
	if (data.robots !== undefined) {
		const robots = record(data.robots, 'metadata.robots', robotsKeys);
		const index = robots.index === undefined ? true : flag(robots.index, 'metadata.robots.index');
		const follow = robots.follow === undefined ? true : flag(robots.follow, 'metadata.robots.follow');
		tags.push(<meta key="robots" name="robots" content={(index ? 'index' : 'noindex') + ',' + (follow ? 'follow' : 'nofollow')} />);
	}
	if (data.openGraph !== undefined) {
		const openGraph = record(data.openGraph, 'metadata.openGraph', openGraphKeys);
		if (openGraph.title !== undefined) tags.push(<meta key="og:title" property="og:title" content={text(openGraph.title, 'metadata.openGraph.title')} />);
		if (openGraph.description !== undefined) tags.push(<meta key="og:description" property="og:description" content={text(openGraph.description, 'metadata.openGraph.description')} />);
		if (openGraph.url !== undefined) tags.push(<meta key="og:url" property="og:url" content={text(openGraph.url, 'metadata.openGraph.url')} />);
		if (openGraph.images !== undefined) list(openGraph.images, 'metadata.openGraph.images').forEach((image, index) => tags.push(<meta key={'og:image:' + index} property="og:image" content={image} />));
	}
	return <>{tags}</>;
}
`

func writeImport(imports *strings.Builder, filePath, name, local string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	if defaultExportPattern.Match(data) {
		fmt.Fprintf(imports, "import %s from %s;\n", local, strconv.Quote(filePath))
		return nil
	}
	fmt.Fprintf(imports, "import { %s as %s } from %s;\n", name, local, strconv.Quote(filePath))
	return nil
}

func hasHeadExport(filePath string) bool {
	data, err := os.ReadFile(filePath)
	return err == nil && headExportPattern.Match(data)
}

func isRouteView(name string) bool {
	switch name {
	case "layout.tsx", "template.tsx", "error.tsx", "not-found.tsx", "loading.tsx":
		return true
	}
	return false
}

func hasMetadataExport(filePath string) bool {
	data, err := os.ReadFile(filePath)
	return err == nil && metadataExportPattern.Match(data)
}

func hasGenerateMetadataExport(filePath string) bool {
	data, err := os.ReadFile(filePath)
	return err == nil && generateMetadataExportPattern.Match(data)
}

func metadataSources(views []string) ([]metadataSource, error) {
	sources := make([]metadataSource, 0, len(views))
	for _, view := range views {
		static, generated := hasMetadataExport(view), hasGenerateMetadataExport(view)
		if static && generated {
			return nil, fmt.Errorf("bifrost: %s exports both metadata and generateMetadata; keep one", filepath.ToSlash(view))
		}
		if static || generated {
			sources = append(sources, metadataSource{View: view, Generate: generated})
		}
	}
	return sources, nil
}

func inheritedWrappers(root, directory string) []viewWrapper {
	var wrappers []viewWrapper
	appendDirectory := func(current string) {
		for _, name := range []string{"layout.tsx", "template.tsx"} {
			path := filepath.Join(root, current, name)
			if fileExists(path) {
				wrappers = append(wrappers, viewWrapper{Path: path, Template: name == "template.tsx"})
			}
		}
	}
	appendDirectory(".")
	if directory != "." {
		current := "."
		for _, part := range strings.Split(directory, "/") {
			current = filepath.Join(current, part)
			appendDirectory(current)
		}
	}
	return wrappers
}

func inheritedFiles(root, directory, name string) []string {
	var files []string
	current := "."
	if fileExists(filepath.Join(root, name)) {
		files = append(files, filepath.Join(root, name))
	}
	if directory == "." {
		return files
	}
	for _, part := range strings.Split(directory, "/") {
		current = filepath.Join(current, part)
		filePath := filepath.Join(root, current, name)
		if fileExists(filePath) {
			files = append(files, filePath)
		}
	}
	return files
}

func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}

func skipDirectory(filePath string, entry os.DirEntry) (bool, error) {
	if !entry.IsDir() {
		return false, nil
	}
	if entry.Name() == ".bifrost" {
		return true, nil
	}
	if strings.HasPrefix(entry.Name(), "_") {
		return true, privateDirectory(filePath)
	}
	return false, nil
}

func privateDirectory(directory string) error {
	var offender string
	err := filepath.WalkDir(directory, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || offender != "" {
			return walkErr
		}
		if !entry.IsDir() && (entry.Name() == "page.tsx" || entry.Name() == "route.go") {
			offender = filePath
		}
		return nil
	})
	if err != nil {
		return err
	}
	if offender == "" {
		return nil
	}
	return fmt.Errorf("bifrost: %s: folders with a leading _ are never routed; rename %s to route it", filepath.ToSlash(offender), filepath.ToSlash(filepath.Dir(offender)))
}

func routePattern(relative string) (string, []routeParam, error) {
	if relative == "." {
		return "/{$}", nil, nil
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	segments := make([]string, 0, len(parts))
	var params []routeParam
	for index, part := range parts {
		segment, param, err := routeSegment(part, index == len(parts)-1)
		if err != nil {
			return "", nil, err
		}
		if param == nil {
			if segment != "" {
				segments = append(segments, segment)
			}
			continue
		}
		if err := appendRouteParam(&params, *param); err != nil {
			return "", nil, err
		}
		segments = append(segments, segment)
	}
	if len(segments) == 0 {
		return "/{$}", params, nil
	}
	return "/" + strings.Join(segments, "/"), params, nil
}

func routeSegment(part string, last bool) (string, *routeParam, error) {
	if err := checkSegmentName(part); err != nil {
		return "", nil, err
	}
	if strings.HasSuffix(part, "~") {
		name := strings.TrimSuffix(part, "~")
		if name == "" || strings.HasSuffix(name, "_") {
			return "", nil, fmt.Errorf("invalid route group %s", part)
		}
		return "", nil, nil
	}
	trailing := len(part) - len(strings.TrimRight(part, "_"))
	if trailing == 0 {
		return part, nil, nil
	}
	if trailing > 2 {
		return "", nil, fmt.Errorf("%s: use one _ for a parameter and __ for the remaining path", part)
	}
	name := strings.TrimRight(part, "_")
	if name == "" {
		return "", nil, fmt.Errorf("invalid parameter %s", part)
	}
	if trailing == 2 && !last {
		return "", nil, fmt.Errorf("the %s parameter must be the last segment", part)
	}
	value := pathValueName(name)
	suffix := ""
	if trailing == 2 {
		suffix = "..."
	}
	return "{" + value + suffix + "}", &routeParam{Name: name, Value: value, Segments: trailing == 2}, nil
}

func appendRouteParam(params *[]routeParam, param routeParam) error {
	for _, existing := range *params {
		if existing.Name == param.Name {
			return fmt.Errorf("duplicate parameter %s", param.Name)
		}
		if existing.Value == param.Value {
			return fmt.Errorf("parameters %s and %s share the name %s in Go", param.Name, existing.Name, param.Value)
		}
	}
	*params = append(*params, param)
	return nil
}

func checkSegmentName(part string) error {
	if part == "" || part == "." || part == ".." {
		return fmt.Errorf("invalid segment %q", part)
	}
	for _, r := range part {
		if goIdentifierRune(r) || r == '-' || r == '.' || r == '+' || r == '~' {
			continue
		}
		switch r {
		case '[', ']':
			return fmt.Errorf("%s: brackets are not valid; name the folder %s", part, parameterFolder(part))
		case '(', ')':
			return fmt.Errorf("%s: parentheses are not valid; name the folder %s~", part, strings.Trim(part, "()"))
		case '@':
			return fmt.Errorf("%s: parallel route folders are not supported", part)
		}
		return fmt.Errorf("invalid character %q in segment %s", r, part)
	}
	return nil
}

func parameterFolder(part string) string {
	name := strings.TrimRight(strings.TrimLeft(part, "["), "]")
	if rest, ok := strings.CutPrefix(name, "..."); ok {
		return rest + "__"
	}
	return name + "_"
}

func pathValueName(name string) string {
	if goIdentifier(name) {
		return name
	}
	var value strings.Builder
	for _, r := range name {
		if goIdentifierRune(r) {
			value.WriteRune(r)
			continue
		}
		value.WriteRune('_')
	}
	if !goIdentifier(value.String()) {
		return "_" + value.String()
	}
	return value.String()
}

func goIdentifier(value string) bool {
	for index, r := range value {
		if !goIdentifierRune(r) {
			return false
		}
		if index == 0 && r >= '0' && r <= '9' {
			return false
		}
	}
	return value != ""
}

func goIdentifierRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return r == '_'
}

// moduleInfo is where the app lives: the module that holds it, the import path
// of the module, and the import path of the route root inside it.
type moduleInfo struct {
	Dir string
	// Root is the import path of the module, e.g. example.com/site.
	Root string
	// Path is the import path of the route root, e.g. example.com/site/app.
	Path string
}

func resolveModule(ctx context.Context, root string, routes []appRoute, hasGo bool) (moduleInfo, error) {
	for _, route := range routes {
		hasGo = hasGo || route.PageGo
	}
	if !hasGo {
		return moduleInfo{}, nil
	}
	command := exec.CommandContext(ctx, "go", "env", "GOMOD")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		return moduleInfo{}, err
	}
	modFile := strings.TrimSpace(string(output))
	if modFile == "" || modFile == os.DevNull {
		return moduleInfo{}, errors.New("bifrost: Go App Router files require an applicable go.mod")
	}
	moduleDir := filepath.Dir(modFile)
	data, err := os.ReadFile(modFile)
	if err != nil {
		return moduleInfo{}, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 || fields[0] != "module" {
		return moduleInfo{}, fmt.Errorf("bifrost: cannot read module path from %s", modFile)
	}
	relativeRoot, err := filepath.Rel(moduleDir, root)
	if err != nil || relativeRoot == ".." || strings.HasPrefix(relativeRoot, ".."+string(filepath.Separator)) {
		return moduleInfo{}, fmt.Errorf("bifrost: app root is outside module %s", moduleDir)
	}
	module := moduleInfo{Dir: moduleDir, Root: fields[1], Path: fields[1]}
	if relativeRoot != "." {
		module.Path += "/" + filepath.ToSlash(relativeRoot)
	}
	return module, nil
}

func importAlias(aliases map[string]string, importPath string) string {
	if alias := aliases[importPath]; alias != "" {
		return alias
	}
	alias := fmt.Sprintf("route%d", len(aliases))
	aliases[importPath] = alias
	return alias
}

func appPackagePaths(modulePath string, routes []appRoute, goDirs []goDir) []string {
	paths := make([]string, 0, len(routes)+len(goDirs))
	join := func(relative string) string {
		if relative == "" || relative == "." {
			return modulePath
		}
		return modulePath + "/" + filepath.ToSlash(relative)
	}
	for _, route := range routes {
		if route.PageGo {
			paths = append(paths, join(route.ImportPath))
		}
		for _, layout := range route.Layouts {
			paths = append(paths, layout.ImportPath)
		}
		for _, layout := range route.ErrorLayouts {
			paths = append(paths, layout.ImportPath)
		}
		if route.Error.ImportPath != "" {
			paths = append(paths, route.Error.ImportPath)
		}
	}
	for _, directory := range goDirs {
		paths = append(paths, join(directory.Directory))
	}
	return paths
}

func checkAppPackages(ctx context.Context, dir string, importPaths []string) error {
	seen := make(map[string]struct{}, len(importPaths))
	unique := make([]string, 0, len(importPaths))
	for _, importPath := range importPaths {
		if _, ok := seen[importPath]; ok {
			continue
		}
		seen[importPath] = struct{}{}
		unique = append(unique, importPath)
	}
	if len(unique) == 0 {
		return nil
	}
	command := exec.CommandContext(ctx, "go", append([]string{"list"}, unique...)...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	return fmt.Errorf("bifrost: the app packages do not load: %s", strings.TrimSpace(string(output)))
}

func hasSymbol(ctx context.Context, dir, importPath, name string) bool {
	command := exec.CommandContext(ctx, "go", "doc", importPath+"."+name)
	command.Dir = dir
	return command.Run() == nil
}

func standardImports() string {
	imports := "\t\"context\"\n\t\"errors\"\n\t\"log\"\n\t\"net/http\"\n\t\"os\"\n\t\"os/signal\"\n\t\"reflect\"\n\t\"strings\"\n"
	return imports + "\t\"syscall\"\n\t\"time\"\n"
}

func paramsLiteral(params []routeParam) string {
	values := make([]string, 0, len(params))
	for _, param := range params {
		value := "r.PathValue(" + strconv.Quote(param.Value) + ")"
		if param.Segments {
			value = "requestSegments(" + value + ")"
		}
		values = append(values, strconv.Quote(param.Name)+": "+value)
	}
	return "map[string]any{" + strings.Join(values, ", ") + "}"
}

func writeRequestProps(loaders *strings.Builder, segments bool) {
	loaders.WriteString("func requestPage(r *http.Request, params map[string]any, props any, fallbacks int) (any, error) {\n\tdata, ok := props.(bifrost.PageData)\n\tif !ok {\n\t\tdata = bifrost.PageData{Props: props}\n\t}\n\tvalues, err := requestProps(data.Props)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\tvalues[\"params\"] = params\n\tvalues[\"searchParams\"] = requestSearchParams(r)\n\tvalues[\"pathname\"] = r.URL.EscapedPath()\n\tdata.Props = values\n\tdata.ErrorFallbacks = fallbacks\n\treturn data, nil\n}\n\n")
	loaders.WriteString("func requestProps(props any) (map[string]any, error) {\n\tif props == nil {\n\t\treturn map[string]any{}, nil\n\t}\n\tvalue := reflect.ValueOf(props)\n\tif value.Kind() != reflect.Map || value.Type().Key().Kind() != reflect.String {\n\t\treturn nil, errors.New(\"bifrost: a page loader must return a map with string keys, or bifrost.PageData with one\")\n\t}\n\tvalues := make(map[string]any, value.Len())\n\titer := value.MapRange()\n\tfor iter.Next() {\n\t\tvalues[iter.Key().String()] = iter.Value().Interface()\n\t}\n\treturn values, nil\n}\n\n")
	loaders.WriteString("func requestSearchParams(r *http.Request) map[string]any {\n\tquery := r.URL.Query()\n\tvalues := make(map[string]any, len(query))\n\tfor key, items := range query {\n\t\tif len(items) == 1 {\n\t\t\tvalues[key] = items[0]\n\t\t\tcontinue\n\t\t}\n\t\tvalues[key] = items\n\t}\n\treturn values\n}\n\n")
	if !segments {
		return
	}
	loaders.WriteString("func requestSegments(value string) []string {\n\tif value == \"\" {\n\t\treturn []string{}\n\t}\n\treturn strings.Split(value, \"/\")\n}\n\n")
}

func writePathValues(builder *strings.Builder, name, handler string, params []routeParam) string {
	var renamed strings.Builder
	for _, param := range params {
		if param.Name == param.Value {
			continue
		}
		fmt.Fprintf(&renamed, "\t\tr.SetPathValue(%s, r.PathValue(%s))\n", strconv.Quote(param.Name), strconv.Quote(param.Value))
	}
	if renamed.Len() == 0 {
		return handler
	}
	fmt.Fprintf(builder, "func %s(next http.Handler) http.Handler {\n\treturn http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {\n%s\t\tnext.ServeHTTP(w, r)\n\t})\n}\n\n", name, renamed.String())
	return name + "(" + handler + ")"
}

func directoryContains(parent, child string) bool {
	return parent == "." || parent == child || strings.HasPrefix(child, parent+"/")
}

// writeNativeRoute emits the render function of a Native route, its layouts
// wrapped around the view, and the declaration that registers it.
func writeNativeRoute(loaders, declarations *strings.Builder, index int, route appRoute, loader string, importsByPath map[string]string, cache string) {
	if !route.ScreenOnly || route.HasMetadata || route.HasSitemap {
		importsByPath[route.ImportPath] = route.Alias
	}
	if !route.ScreenOnly {
		for _, layout := range route.Layouts {
			importsByPath[layout.ImportPath] = layout.Alias
		}
	}
	links := "nil"
	if route.HasLinks {
		links = route.Alias + ".Links"
	}
	viewName := route.ViewName
	if viewName == "" {
		viewName = "View"
	}
	var view string
	switch {
	case route.GoNotFound:
		view = route.Alias + "." + viewName + "()"
	case route.ScreenOnly:
	default:
		arguments := ""
		if route.ViewArgs > 0 {
			arguments = "parsed"
		}
		view = route.Alias + "." + viewName + "(" + arguments + ")"
	}
	if view != "" {
		for _, layout := range route.Layouts {
			view = layout.Alias + ".Layout(" + view + ")"
		}
	}
	source := "web.Shell(nil, shellOptions...)"
	if route.ScreenOnly {
		// A screen has no content to keep: the spinner shows at once.
		source = "web.Shell(nil, append(shellOptions, web.Screen())...)"
	}
	if view != "" {
		source = fmt.Sprintf("web.Shell(seo.Body(%s, seo.Options{Links: %s}), shellOptions...)", view, links)
	}
	cached := route.ViewArgs == 0
	if cached {
		fmt.Fprintf(loaders, "var body%d = sync.OnceValue(func() []byte {\n\treturn %s\n})\n\n", index, source)
	}
	fmt.Fprintf(loaders, "func render%d(ctx context.Context, props json.RawMessage) (bifrost.NativePage, error) {\n", index)
	needsProps := route.HasProps && (route.ViewArgs > 0 || route.MetadataArgs > 0)
	if needsProps {
		fmt.Fprintf(loaders, "\tvar parsed %s.Props\n\tif len(props) > 0 {\n\t\tif err := json.Unmarshal(props, &parsed); err != nil {\n\t\t\treturn bifrost.NativePage{}, err\n\t\t}\n\t}\n", route.Alias)
	}
	if cached {
		fmt.Fprintf(loaders, "\tpage := bifrost.NativePage{Body: body%d()}\n", index)
	} else {
		fmt.Fprintf(loaders, "\tpage := bifrost.NativePage{Body: %s}\n", source)
	}
	if route.HasMetadata {
		metadataName := route.MetadataName
		if metadataName == "" {
			metadataName = "Metadata"
		}
		arguments := ""
		if route.MetadataArgs > 0 {
			arguments = "parsed"
		}
		fmt.Fprintf(loaders, "\tmeta := %s.%s(%s)\n", route.Alias, metadataName, arguments)
		loaders.WriteString("\tpage.Title, page.Description, page.Canonical = meta.Title, meta.Description, meta.Canonical\n")
		loaders.WriteString("\tpage.Image, page.Robots, page.Schema, page.Lang, page.Dir = meta.Image, meta.Robots, meta.Schema, meta.Lang, meta.Dir\n")
		loaders.WriteString("\tpage.Keywords, page.Type = meta.Keywords, meta.Type\n")
		loaders.WriteString("\tfor _, alternate := range meta.Alternates {\n\t\tpage.Alternates = append(page.Alternates, bifrost.Alternate{Lang: alternate.Lang, URL: alternate.URL})\n\t}\n")
	}
	if route.ScreenOnly {
		loaders.WriteString("\tif page.Robots == \"\" {\n\t\tpage.Robots = \"noindex\"\n\t}\n")
	}
	loaders.WriteString("\treturn page, nil\n}\n\n")
	suffix := cache
	if route.Error.ImportPath != "" {
		importsByPath[route.Error.ImportPath] = route.Error.Alias
		for _, layout := range route.ErrorLayouts {
			importsByPath[layout.ImportPath] = layout.Alias
		}
		errorLinks := "nil"
		if route.ErrorLinks {
			errorLinks = route.Error.Alias + ".Links"
		}
		errorView := route.Error.Alias + ".Error(err)"
		for _, layout := range route.ErrorLayouts {
			errorView = layout.Alias + ".Layout(" + errorView + ")"
		}
		fmt.Fprintf(loaders, "func renderError%d(ctx context.Context, err error) (bifrost.NativePage, error) {\n", index)
		if route.HasNotFound && !route.GoNotFound {
			fmt.Fprintf(loaders, "\tif status, ok := bifrost.ErrorStatus(err); ok && status == http.StatusNotFound {\n\t\treturn render%d(ctx, nil)\n\t}\n", route.NotFoundAt)
		}
		fmt.Fprintf(loaders, "\tpage := bifrost.NativePage{Body: web.Shell(seo.Body(%s, seo.Options{Links: %s}), shellOptions...)}\n", errorView, errorLinks)
		if route.ErrorMeta {
			fmt.Fprintf(loaders, "\tmeta := %s.ErrorMetadata()\n", route.Error.Alias)
			loaders.WriteString("\tpage.Title, page.Description, page.Canonical = meta.Title, meta.Description, meta.Canonical\n")
			loaders.WriteString("\tpage.Image, page.Robots, page.Schema, page.Lang, page.Dir = meta.Image, meta.Robots, meta.Schema, meta.Lang, meta.Dir\n")
			loaders.WriteString("\tpage.Keywords, page.Type = meta.Keywords, meta.Type\n")
			loaders.WriteString("\tfor _, alternate := range meta.Alternates {\n\t\tpage.Alternates = append(page.Alternates, bifrost.Alternate{Lang: alternate.Lang, URL: alternate.URL})\n\t}\n")
		}
		loaders.WriteString("\tif page.Robots == \"\" {\n\t\tpage.Robots = \"noindex\"\n\t}\n")
		loaders.WriteString("\treturn page, nil\n}\n\n")
		suffix += fmt.Sprintf(".WithError(renderError%d)", index)
	}
	fmt.Fprintf(declarations, "\t\t\tbifrost.Native(%s, render%d, %s)%s,\n", strconv.Quote(route.Pattern), index, loader, suffix)
}

// nearestNotFound returns the not-found page of the directory, or of the
// nearest one above it, which is the page a load that answers 404 shows.
func nearestNotFound(notFoundAt map[string]int, directory string) (int, bool) {
	for {
		if at, ok := notFoundAt[directory]; ok {
			return at, true
		}
		if directory == "." {
			return 0, false
		}
		if index := strings.LastIndex(directory, "/"); index >= 0 {
			directory = directory[:index]
			continue
		}
		directory = "."
	}
}

// hasCachedBody reports whether a page builds its body once: a view that takes
// no props draws the same page for every request, and the renderer runs it once.
func hasCachedBody(routes []appRoute) bool {
	return slices.ContainsFunc(routes, func(route appRoute) bool { return route.Native && route.ViewArgs == 0 })
}

// nativeSupport is what the generated server shares between its pages: the
// build the browser boots from, and the shell a dev server makes the page
// follow.
func nativeSupport(routes []appRoute, browserBuild bool) string {
	if !slices.ContainsFunc(routes, func(route appRoute) bool { return route.Native }) {
		return ""
	}
	var support strings.Builder
	support.WriteString("var shellOptions = shellOptionsFromEnv()\n\nfunc shellOptionsFromEnv() []web.Option {\n\tif os.Getenv(\"BIFROST_DEV\") != \"\" {\n\t\treturn []web.Option{web.Reload()}\n\t}\n\treturn nil\n}\n\n")
	if browserBuild {
		support.WriteString("//go:embed all:static\nvar bifrostStatic embed.FS\n\nfunc wasmSource() fs.FS {\n\tif dir := os.Getenv(\"BIFROST_WASM_DIR\"); dir != \"\" {\n\t\treturn os.DirFS(dir)\n\t}\n\tfiles, err := fs.Sub(bifrostStatic, \"static\")\n\tif err != nil {\n\t\tpanic(err)\n\t}\n\treturn files\n}\n\n")
	}
	return support.String()
}

func writeAppRouterMain(root, generated string, routes []appRoute, goDirs []goDir, renderConcurrency, renderQueue int, commandsImport string) error {
	browserBuild := slices.ContainsFunc(routes, func(route appRoute) bool { return route.Native }) && packageDeclaresType(root, "app")
	renderConfig := ""
	if slices.ContainsFunc(routes, func(route appRoute) bool { return !route.Native }) {
		renderConfig = "Assets: bifrostAssets, "
	}
	if renderConcurrency > 0 {
		renderConfig += "RenderConcurrency: " + strconv.Itoa(renderConcurrency) + ", "
	}
	if renderQueue > 0 {
		renderConfig += "RenderQueue: " + strconv.Itoa(renderQueue) + ", "
	}
	importsByPath := make(map[string]string)
	var declarations strings.Builder
	var loaders strings.Builder
	hasNotFoundPage := false
	hasSegments := false
	notFoundAt := make(map[string]int)
	for index, route := range routes {
		if route.GoNotFound {
			notFoundAt[route.Directory] = index
		}
	}
	for index := range routes {
		if at, ok := nearestNotFound(notFoundAt, routes[index].Directory); ok {
			routes[index].NotFoundAt, routes[index].HasNotFound = at, true
		}
	}
	for index, route := range routes {
		if route.NotFoundPage {
			hasNotFoundPage = true
			fmt.Fprintf(&declarations, "\t\t\tbifrost.Server(%s, %s, loadNotFound).WithNavigation(),\n", strconv.Quote(route.Pattern), strconv.Quote(route.View))
			continue
		}
		if route.Native {
			cache := ""
			if route.HasCache {
				cache = ".WithCache(" + route.Alias + ".Cache())"
			}
			if route.ScreenOnly {
				cache += ".WithNoIndex()"
			}
			if route.HasSitemap {
				if route.SitemapKind == sitemapEntries {
					cache += ".WithSitemapEntries(" + route.Alias + ".Sitemap()...)"
				} else {
					cache += ".WithSitemap(" + route.Alias + ".Sitemap()...)"
				}
			}
			loader := "nil"
			switch {
			case route.GoNotFound:
				name := fmt.Sprintf("loadGoNotFound%d", index)
				fmt.Fprintf(&loaders, "func %s(r *http.Request) (any, error) {\n\treturn bifrost.PageData{Status: http.StatusNotFound}, nil\n}\n\n", name)
				loader = name
			case route.HasLoader:
				importsByPath[route.ImportPath] = route.Alias
				name := fmt.Sprintf("load%d", index)
				fmt.Fprintf(&loaders, "func %s(r *http.Request) (any, error) {\n\treturn %s.Load(r)\n}\n\n", name, route.Alias)
				loader = name
			}
			writeNativeRoute(&loaders, &declarations, index, route, loader, importsByPath, cache)
			continue
		}
		hasSegments = hasSegments || slices.ContainsFunc(route.Params, func(param routeParam) bool { return param.Segments })
		loader := fmt.Sprintf("load%d", index)
		params := paramsLiteral(route.Params)
		if route.HasLoader {
			importsByPath[route.ImportPath] = route.Alias
			fmt.Fprintf(&loaders, "func %s(r *http.Request) (any, error) {\n\tprops, err := %s.Load(r)\n\tif err == nil {\n\t\treturn requestPage(r, %s, props, %d)\n\t}\n\tif bifrost.IsRedirect(err) {\n\t\treturn nil, err\n\t}\n", loader, route.Alias, params, len(route.ErrorViews))
			if len(route.ErrorViews) == 0 && route.NotFoundView == "" {
				fmt.Fprintf(&loaders, "\treturn nil, err\n")
			} else {
				fmt.Fprintf(&loaders, "\tstatus, ok := bifrost.ErrorStatus(err)\n\tif !ok {\n\t\tstatus = http.StatusInternalServerError\n\t}\n")
				if route.NotFoundView != "" {
					fmt.Fprintf(&loaders, "\tif status == http.StatusNotFound {\n\t\treturn requestPage(r, %s, bifrost.PageData{Props: map[string]any{\"__bifrostNotFound\": true}, Status: status}, %d)\n\t}\n", params, len(route.ErrorViews))
				}
				if len(route.ErrorViews) > 0 {
					fmt.Fprintf(&loaders, "\tmessage := http.StatusText(status)\n\tif os.Getenv(\"BIFROST_DEV_DIR\") != \"\" {\n\t\tmessage = err.Error()\n\t}\n\treturn requestPage(r, %s, bifrost.PageData{Props: map[string]any{\"__bifrostError\": message, \"__bifrostErrorLevel\": %d}, Status: status}, %d)\n", params, len(route.ErrorViews)-1, len(route.ErrorViews))
				} else {
					fmt.Fprintf(&loaders, "\treturn nil, err\n")
				}
			}
			fmt.Fprintf(&loaders, "}\n\n")
		} else {
			fmt.Fprintf(&loaders, "func %s(r *http.Request) (any, error) {\n\treturn requestPage(r, %s, nil, %d)\n}\n\n", loader, params, len(route.ErrorViews))
		}
		cache := ""
		if route.HasCache {
			cache = ".WithCache(" + route.Alias + ".Cache())"
		}
		fmt.Fprintf(&declarations, "\t\t\tbifrost.Server(%s, %s, %s).WithNavigation()%s,\n", strconv.Quote(route.Pattern), strconv.Quote(route.View), loader, cache)
	}
	if hasNotFoundPage {
		fmt.Fprintf(&loaders, "func loadNotFound(r *http.Request) (any, error) {\n\treturn bifrost.PageData{Props: map[string]any{\"pathname\": r.URL.EscapedPath()}, Status: http.StatusNotFound}, nil\n}\n\n")
	}
	writeRequestProps(&loaders, hasSegments)
	for _, directory := range goDirs {
		if directory.Middleware || directory.Serve || len(directory.HTTPMethods) > 0 {
			importsByPath[directory.ImportPath] = directory.Alias
		}
	}
	nativeImports := ""
	wasmAssets := ""
	extraImports := ""
	if slices.ContainsFunc(routes, func(route appRoute) bool { return route.Native }) {
		nativeImports = "\t\"github.com/3-lines-studio/bifrost/mygo/web\"\n"
		if strings.Contains(loaders.String(), "seo.") {
			// Only a page or an error page with a view renders one.
			nativeImports = "\t\"github.com/3-lines-studio/bifrost/mygo/seo\"\n" + nativeImports
		}
		extraImports += "\t\"encoding/json\"\n"
	}
	if hasCachedBody(routes) {
		extraImports += "\t\"sync\"\n"
	}
	if browserBuild {
		extraImports += "\t\"embed\"\n\t\"io/fs\"\n"
		wasmAssets = "\twasmFiles := web.FileServerFS(wasmSource())\n\tmux.Handle(\"GET /app.wasm\", wasmFiles)\n\tmux.Handle(\"GET /wasm_exec.js\", wasmFiles)\n\tmux.Handle(\"GET \"+web.ReloadPath, web.ReloadHandler())\n"
	}
	paths := make([]string, 0, len(importsByPath))
	for importPath := range importsByPath {
		paths = append(paths, importPath)
	}
	slices.Sort(paths)
	var imports strings.Builder
	for _, importPath := range paths {
		fmt.Fprintf(&imports, "\t%s %s\n", importsByPath[importPath], strconv.Quote(importPath))
	}
	var registrations strings.Builder
	var pathValues strings.Builder
	for index, route := range routes {
		handler := "http.Handler(pageMux)"
		for index := len(goDirs) - 1; index >= 0; index-- {
			if goDirs[index].Middleware && directoryContains(goDirs[index].Directory, route.Directory) {
				handler = goDirs[index].Alias + ".Middleware(" + handler + ")"
			}
		}
		handler = writePathValues(&pathValues, fmt.Sprintf("pathValues%d", index), handler, route.Params)
		fmt.Fprintf(&registrations, "\tmux.Handle(%s, %s)\n", strconv.Quote("GET "+route.Pattern), handler)
	}
	for directoryIndex, directory := range goDirs {
		for _, method := range directory.HTTPMethods {
			handler := "http.HandlerFunc(" + directory.Alias + "." + method + ")"
			for index := len(goDirs) - 1; index >= 0; index-- {
				if goDirs[index].Middleware && directoryContains(goDirs[index].Directory, directory.Directory) {
					handler = goDirs[index].Alias + ".Middleware(" + handler + ")"
				}
			}
			handler = writePathValues(&pathValues, fmt.Sprintf("pathValuesGo%d", directoryIndex), handler, directory.Params)
			fmt.Fprintf(&registrations, "\tmux.Handle(%s, %s)\n", strconv.Quote(strings.ToUpper(method)+" "+directory.Pattern), handler)
		}
	}
	serve := "return serve(ctx, redirectTrailingSlash(app.ResolveMarkdown(mux)))"
	for _, directory := range goDirs {
		if directory.Serve {
			serve = "return " + directory.Alias + ".Serve(ctx, redirectTrailingSlash(app.ResolveMarkdown(mux)))"
			break
		}
	}
	commands := ""
	if commandsImport != "" {
		imports.WriteString("\tappcommands " + strconv.Quote(commandsImport) + "\n")
		// A one-off command runs before the app exists: no renderer, no Bun, no
		// port. That is what makes `./app migrate` or a cron job cheap.
		commands = "\tif len(os.Args) > 1 {\n\t\tctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)\n\t\tdefer stop()\n\t\thandled, err := appcommands.Run(ctx, os.Args[1:])\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\tif handled {\n\t\t\treturn nil\n\t\t}\n\t}\n"
	}
	source := "package main\n\nimport (\n" + standardImports() + extraImports + "\n\t\"github.com/3-lines-studio/bifrost\"\n" + nativeImports + imports.String() + ")\n\n" + nativeSupport(routes, browserBuild) + loaders.String() + pathValues.String() + "func main() {\n\tif err := run(); err != nil {\n\t\tlog.Fatal(err)\n\t}\n}\n\nfunc run() error {\n" + commands + "\tapp, err := bifrost.New(bifrost.Config{SourceRoot: " + strconv.Quote(root) + ", SiteURL: os.Getenv(\"BIFROST_SITE_URL\"), " + renderConfig + "Routes: []bifrost.Route{\n" + declarations.String() + "\t}})\n\tif err != nil {\n\t\treturn err\n\t}\n\tif bifrost.Building() {\n\t\treturn nil\n\t}\n\tctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)\n\tdefer stop()\n\tdefer func() {\n\t\tcloseCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)\n\t\tdefer cancel()\n\t\t_ = app.Close(closeCtx)\n\t}()\n\tpageMux := http.NewServeMux()\n\tif err := app.Register(pageMux); err != nil {\n\t\treturn err\n\t}\n\tmux := http.NewServeMux()\n" + wasmAssets + registrations.String() + "\tmux.Handle(\"/\", pageMux)\n\t" + serve + "\n}\n\nfunc redirectTrailingSlash(next http.Handler) http.Handler {\n\treturn http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {\n\t\tpath := r.URL.Path\n\t\tif len(path) < 2 || !strings.HasSuffix(path, \"/\") {\n\t\t\tnext.ServeHTTP(w, r)\n\t\t\treturn\n\t\t}\n\t\ttarget := *r.URL\n\t\ttarget.Path = strings.TrimSuffix(path, \"/\")\n\t\thttp.Redirect(w, r, target.String(), http.StatusPermanentRedirect)\n\t})\n}\n\nfunc serve(ctx context.Context, handler http.Handler) error {\n\treturn bifrost.Serve(ctx, handler)\n}"
	formatted, err := format.Source([]byte(source))
	if err != nil {
		return err
	}
	if browserBuild {
		if err := os.MkdirAll(filepath.Join(generated, "static"), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(generated, "static", ".keep"), []byte("the browser build, copied here by bifrost build\n"), 0o644); err != nil {
			return err
		}
	}
	path := filepath.Join(generated, "main.go")
	current, _ := os.ReadFile(path)
	if bytes.Equal(current, formatted) {
		return nil
	}
	return os.WriteFile(path, formatted, 0o644)
}

func writeAppRoutes(projectRoot string, routes []appRoute) error {
	path := filepath.Join(projectRoot, "zz_bifrost_routes.go")
	selected := make([]appRoute, 0, len(routes))
	for _, route := range routes {
		if route.Native && route.Screen {
			selected = append(selected, route)
		}
	}
	if len(selected) == 0 || !packageDeclaresType(projectRoot, "app") {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	aliases := map[string]string{"github.com/egoist/mygo/ui": "ui"}
	for index := range selected {
		selected[index].Alias = importAlias(aliases, selected[index].ImportPath)
		for layoutIndex := range selected[index].Layouts {
			layout := &selected[index].Layouts[layoutIndex]
			layout.Alias = importAlias(aliases, layout.ImportPath)
		}
	}
	paths := make([]string, 0, len(aliases))
	for importPath := range aliases {
		if importPath != "github.com/egoist/mygo/ui" {
			paths = append(paths, importPath)
		}
	}
	slices.Sort(paths)

	var source strings.Builder
	source.WriteString("// Code generated by bifrost. DO NOT EDIT.\n\npackage main\n\nimport (\n\t\"github.com/egoist/mygo/ui\"\n")
	for _, importPath := range paths {
		fmt.Fprintf(&source, "\t%s %s\n", aliases[importPath], strconv.Quote(importPath))
	}
	source.WriteString(")\n\ntype appPage struct {\n\tPattern string\n\tView    func(*ui.Context)\n}\n\nfunc appPages(app *app) []appPage {\n\treturn []appPage{\n")
	for _, route := range selected {
		view := route.Alias + ".Screen()"
		if route.ScreenArgs > 0 {
			view = route.Alias + ".Screen(app)"
		}
		for _, layout := range route.Layouts {
			view = layout.Alias + ".Layout(" + view + ")"
		}
		fmt.Fprintf(&source, "\t\t{Pattern: %s, View: %s},\n", strconv.Quote(routerPattern(route.Pattern)), view)
	}
	source.WriteString("\t}\n}\n")
	formatted, err := format.Source([]byte(source.String()))
	if err != nil {
		return err
	}
	return os.WriteFile(path, formatted, 0o644)
}

func routerPattern(pattern string) string {
	if pattern == "/{$}" {
		return "/"
	}
	if before, ok := strings.CutSuffix(pattern, "{$}"); ok {
		return before
	}
	return pattern
}

func writeAppRouterModule(generated, userModuleDir string, nativePages bool) error {
	version := bifrost.Version
	bifrostModuleDir := ""
	if developmentVersion(version) {
		_, source, _, ok := runtime.Caller(0)
		if !ok {
			return errors.New("bifrost: cannot locate the development module")
		}
		bifrostModuleDir = filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
		version = "v0.0.0"
	}

	var module strings.Builder
	module.WriteString("module bifrost.local/app\n\ngo 1.25.0\n\nrequire github.com/3-lines-studio/bifrost ")
	module.WriteString(version)
	module.WriteString("\n")

	userModulePath := ""
	var userRequires, userReplaces []string
	if userModuleDir != "" {
		data, err := os.ReadFile(filepath.Join(userModuleDir, "go.mod"))
		if err != nil {
			return err
		}
		fields := strings.Fields(string(data))
		if len(fields) < 2 || fields[0] != "module" {
			return fmt.Errorf("bifrost: cannot read module path from %s", filepath.Join(userModuleDir, "go.mod"))
		}
		userModulePath = fields[1]
		if userModulePath == "github.com/3-lines-studio/bifrost" {
			bifrostModuleDir = userModuleDir
		}
		userRequires, userReplaces = moduleDirectives(string(data), userModuleDir)
	}

	requires := make([]string, 0, len(userRequires)+1)
	hasAdapter := false
	for _, entry := range userRequires {
		path := strings.Fields(entry)
		if len(path) == 0 || path[0] == "github.com/3-lines-studio/bifrost" || path[0] == userModulePath {
			continue
		}
		hasAdapter = hasAdapter || path[0] == "github.com/3-lines-studio/bifrost/mygo"
		requires = append(requires, entry)
	}
	if nativePages && !hasAdapter {
		if bifrostModuleDir == "" {
			return errors.New("bifrost: add github.com/3-lines-studio/bifrost/mygo to the module for Go pages")
		}
		requires = append(requires, "github.com/3-lines-studio/bifrost/mygo v0.0.0")
	}
	if userModulePath != "" {
		requires = append(requires, userModulePath+" v0.0.0")
	}
	for _, entry := range requires {
		module.WriteString("require ")
		module.WriteString(entry)
		module.WriteString("\n")
	}

	replaces := slices.Clone(userReplaces)
	replaced := func(path string) bool {
		for _, entry := range replaces {
			if strings.HasPrefix(entry, path+" ") || strings.HasPrefix(entry, path+" =>") {
				return true
			}
		}
		return false
	}
	if userModulePath != "" {
		replaces = append(replaces, userModulePath+" => "+strconv.Quote(filepath.ToSlash(userModuleDir)))
	}
	if bifrostModuleDir != "" && !replaced("github.com/3-lines-studio/bifrost") {
		replaces = append(replaces, "github.com/3-lines-studio/bifrost => "+strconv.Quote(filepath.ToSlash(bifrostModuleDir)))
	}
	mygoModuleDir := ""
	if bifrostModuleDir != "" {
		candidate := filepath.Join(bifrostModuleDir, "mygo")
		if _, err := os.Stat(filepath.Join(candidate, "go.mod")); err == nil {
			mygoModuleDir = candidate
		}
	}
	if mygoModuleDir != "" && !replaced("github.com/3-lines-studio/bifrost/mygo") {
		replaces = append(replaces, "github.com/3-lines-studio/bifrost/mygo => "+strconv.Quote(filepath.ToSlash(mygoModuleDir)))
	}
	for _, entry := range replaces {
		module.WriteString("replace ")
		module.WriteString(entry)
		module.WriteString("\n")
	}
	return os.WriteFile(filepath.Join(generated, "go.mod"), []byte(module.String()), 0o644)
}

func moduleDirectives(data, userModuleDir string) (requires, replaces []string) {
	block := ""
	for _, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if block != "" {
			if trimmed == ")" {
				block = ""
				continue
			}
			if block == "require" {
				requires = append(requires, trimmed)
			} else {
				replaces = append(replaces, resolveReplace(trimmed, userModuleDir))
			}
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "require ("):
			block = "require"
		case strings.HasPrefix(trimmed, "replace ("):
			block = "replace"
		case strings.HasPrefix(trimmed, "require "):
			requires = append(requires, strings.TrimSpace(strings.TrimPrefix(trimmed, "require ")))
		case strings.HasPrefix(trimmed, "replace "):
			replaces = append(replaces, resolveReplace(strings.TrimSpace(strings.TrimPrefix(trimmed, "replace ")), userModuleDir))
		}
	}
	return requires, replaces
}

func resolveReplace(entry, userModuleDir string) string {
	parts := strings.SplitN(entry, "=>", 2)
	if len(parts) != 2 {
		return entry
	}
	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])
	if !strings.HasPrefix(right, ".") {
		return left + " => " + right
	}
	target := right
	version := ""
	if fields := strings.Fields(right); len(fields) > 1 {
		target, version = fields[0], " "+fields[1]
	}
	return left + " => " + strconv.Quote(filepath.ToSlash(filepath.Join(userModuleDir, target))) + version
}

func buildAppRouter(ctx context.Context, app appRouter, options builder.Options, buildExecutable bool) error {
	options.Package = app.Package
	options.Dir = app.WorkDir
	options.Output = app.Output
	options.RouteParams = app.RouteParams
	if !app.NativeOnly {
		if err := builder.Build(ctx, options); err != nil {
			return err
		}
	}
	if !buildExecutable {
		return nil
	}
	if app.NativePages {
		if err := buildWasm(ctx, app.ProjectRoot, true); err != nil {
			return err
		}
		if err := copyWasmStatic(app.ProjectRoot); err != nil {
			return err
		}
	}
	command := exec.CommandContext(ctx, "go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w", "-o", app.Executable, app.Package)
	command.Dir = app.WorkDir
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}
