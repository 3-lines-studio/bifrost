package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/3-lines-studio/bifrost"
	"github.com/3-lines-studio/bifrost/internal/builder"
)

type conventionRoute struct {
	Directory    string
	Pattern      string
	View         string
	ImportPath   string
	Alias        string
	PageGo       bool
	HasLoader    bool
	HasHead      bool
	ErrorViews   []string
	NotFoundView string
	NotFoundPage bool
}

type conventionGoDir struct {
	Directory   string
	Pattern     string
	ImportPath  string
	Alias       string
	Middleware  bool
	Serve       bool
	HTTPMethods []string
}

func conventionDirectory(dir, packagePath string) bool {
	root := packagePath
	if !filepath.IsAbs(root) {
		root = filepath.Join(dir, root)
	}
	info, err := os.Stat(filepath.Join(root, "page.tsx"))
	return err == nil && !info.IsDir()
}

func conventionRoot(dir, packagePath string) string {
	if filepath.IsAbs(packagePath) {
		return packagePath
	}
	return filepath.Join(dir, packagePath)
}

type conventionApp struct {
	Root       string
	WorkDir    string
	Package    string
	Output     string
	Executable string
}

func prepareConventionApp(ctx context.Context, dir string) (conventionApp, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return conventionApp{}, err
	}
	routes, err := discoverConventionRoutes(root)
	if err != nil {
		return conventionApp{}, err
	}
	if len(routes) == 0 {
		return conventionApp{}, fmt.Errorf("bifrost: no page.tsx found under %s", root)
	}
	routes, err = appendNotFoundRoutes(root, routes)
	if err != nil {
		return conventionApp{}, err
	}
	goDirs, err := discoverConventionGo(root)
	if err != nil {
		return conventionApp{}, err
	}
	modulePath, moduleDir, err := conventionModule(ctx, root, routes, len(goDirs) > 0)
	if err != nil {
		return conventionApp{}, err
	}
	generated := filepath.Join(root, ".bifrost", "app")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		return conventionApp{}, err
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
		routes[index].HasLoader = hasSymbol(ctx, moduleDir, routes[index].ImportPath, "Load")
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
			return conventionApp{}, fmt.Errorf("bifrost: server.go is only valid at the app root")
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
			routes[index].Alias = conventionAlias(aliases, routes[index].ImportPath)
		}
	}
	for index := range goDirs {
		goDirs[index].Alias = conventionAlias(aliases, goDirs[index].ImportPath)
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
				return conventionApp{}, fmt.Errorf("bifrost: %s in %s/route.go conflicts with page.tsx", method, directory.Directory)
			}
		}
	}
	if err := writeConventionViews(root, routes); err != nil {
		return conventionApp{}, err
	}
	if err := writeConventionMain(root, generated, routes, goDirs); err != nil {
		return conventionApp{}, err
	}
	if err := writeConventionModule(generated, moduleDir); err != nil {
		return conventionApp{}, err
	}
	if err := os.MkdirAll(filepath.Join(generated, "build"), 0o755); err != nil {
		return conventionApp{}, err
	}
	if err := os.WriteFile(filepath.Join(generated, "build", "embed.placeholder"), nil, 0o644); err != nil {
		return conventionApp{}, err
	}
	command := exec.CommandContext(ctx, "go", "mod", "tidy")
	command.Dir = generated
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return conventionApp{}, fmt.Errorf("bifrost: prepare generated module: %w", err)
	}
	return conventionApp{Root: root, WorkDir: generated, Package: ".", Output: filepath.Join(generated, "build"), Executable: filepath.Join(root, ".bifrost", "bifrost-app")}, nil
}

func discoverConventionRoutes(root string) ([]conventionRoute, error) {
	var routes []conventionRoute
	patterns := make(map[string]string)
	err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == ".bifrost" {
			return filepath.SkipDir
		}
		if entry.IsDir() || entry.Name() != "page.tsx" {
			return nil
		}
		directory := filepath.Dir(filePath)
		relative, err := filepath.Rel(root, directory)
		if err != nil {
			return err
		}
		pattern, err := conventionPattern(relative)
		if err != nil {
			return fmt.Errorf("bifrost: route %s: %w", filepath.ToSlash(relative), err)
		}
		if previous := patterns[pattern]; previous != "" {
			return fmt.Errorf("bifrost: duplicate route %q from %s and %s", pattern, previous, filepath.ToSlash(relative))
		}
		patterns[pattern] = filepath.ToSlash(relative)
		route := conventionRoute{Directory: filepath.ToSlash(relative), Pattern: pattern, View: filepath.ToSlash(strings.TrimPrefix(filePath, root+string(filepath.Separator))), HasHead: hasHeadExport(filePath)}
		if _, err := os.Stat(filepath.Join(directory, "page.go")); err == nil {
			route.PageGo = true
			route.ImportPath = filepath.ToSlash(relative)
			if relative == "." {
				route.ImportPath = ""
			}
		}
		routes = append(routes, route)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(routes, func(a, b conventionRoute) int { return strings.Compare(a.Pattern, b.Pattern) })
	return routes, nil
}

func appendNotFoundRoutes(root string, routes []conventionRoute) ([]conventionRoute, error) {
	err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == ".bifrost" {
			return filepath.SkipDir
		}
		if entry.IsDir() || entry.Name() != "not-found.tsx" {
			return nil
		}
		relative, err := filepath.Rel(root, filepath.Dir(filePath))
		if err != nil {
			return err
		}
		pattern := "/{path...}"
		if relative != "." {
			base, err := conventionPattern(relative)
			if err != nil {
				return err
			}
			pattern = base + "/{path...}"
		}
		routes = append(routes, conventionRoute{Directory: filepath.ToSlash(relative), Pattern: pattern, View: filepath.ToSlash(strings.TrimPrefix(filePath, root+string(filepath.Separator))), NotFoundPage: true})
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(routes, func(a, b conventionRoute) int { return strings.Compare(a.Pattern, b.Pattern) })
	return routes, nil
}

func discoverConventionGo(root string) ([]conventionGoDir, error) {
	byDirectory := make(map[string]conventionGoDir)
	err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == ".bifrost" {
			return filepath.SkipDir
		}
		if entry.IsDir() || entry.Name() != "route.go" && entry.Name() != "middleware.go" && entry.Name() != "server.go" {
			return nil
		}
		relative, err := filepath.Rel(root, filepath.Dir(filePath))
		if err != nil {
			return err
		}
		pattern, err := conventionPattern(relative)
		if err != nil {
			return fmt.Errorf("bifrost: route %s: %w", filepath.ToSlash(relative), err)
		}
		directory := filepath.ToSlash(relative)
		item := byDirectory[directory]
		item.Directory = directory
		item.Pattern = pattern
		byDirectory[directory] = item
		return nil
	})
	if err != nil {
		return nil, err
	}
	items := make([]conventionGoDir, 0, len(byDirectory))
	for _, item := range byDirectory {
		items = append(items, item)
	}
	slices.SortFunc(items, func(a, b conventionGoDir) int { return strings.Compare(a.Directory, b.Directory) })
	return items, nil
}

func writeConventionViews(root string, routes []conventionRoute) error {
	views := filepath.Join(root, ".bifrost", "views")
	if err := os.RemoveAll(views); err != nil {
		return err
	}
	if err := os.MkdirAll(views, 0o755); err != nil {
		return err
	}
	for index := range routes {
		layouts := inheritedFiles(root, routes[index].Directory, "layout.tsx")
		errors := inheritedFiles(root, routes[index].Directory, "error.tsx")
		notFound := inheritedFiles(root, routes[index].Directory, "not-found.tsx")
		if routes[index].NotFoundPage {
			errors = nil
			notFound = nil
		}
		routes[index].ErrorViews = errors
		if len(notFound) > 0 {
			routes[index].NotFoundView = notFound[len(notFound)-1]
		}
		if len(layouts) == 0 && len(errors) == 0 && len(notFound) == 0 && !routes[index].NotFoundPage {
			continue
		}
		var imports strings.Builder
		export := "Page"
		if routes[index].NotFoundPage {
			export = "NotFound"
		}
		fmt.Fprintf(&imports, "import { %s as RoutePage } from %s;\n", export, strconv.Quote(filepath.Join(root, filepath.FromSlash(routes[index].View))))
		for layoutIndex, layout := range layouts {
			fmt.Fprintf(&imports, "import { Layout as Layout%d } from %s;\n", layoutIndex, strconv.Quote(layout))
		}
		for errorIndex, errorView := range routes[index].ErrorViews {
			fmt.Fprintf(&imports, "import { Error as ErrorPage%d } from %s;\n", errorIndex, strconv.Quote(errorView))
		}
		if routes[index].NotFoundView != "" {
			fmt.Fprintf(&imports, "import { NotFound } from %s;\n", strconv.Quote(routes[index].NotFoundView))
		}
		body := "<RoutePage {...props} />"
		if routes[index].NotFoundPage {
			body = "<RoutePage />"
		}
		for errorIndex := range routes[index].ErrorViews {
			body = fmt.Sprintf("props.__bifrostError && props.__bifrostErrorLevel === %d ? <ErrorPage%d error={String(props.__bifrostError)} /> : %s", errorIndex, errorIndex, body)
		}
		if routes[index].NotFoundView != "" {
			body = "props.__bifrostNotFound ? <NotFound /> : " + body
		}
		if len(layouts) > 0 && strings.Contains(body, " ? ") {
			body = "{" + body + "}"
		}
		for layoutIndex := len(layouts) - 1; layoutIndex >= 0; layoutIndex-- {
			body = fmt.Sprintf("<Layout%d>%s</Layout%d>", layoutIndex, body, layoutIndex)
		}
		head := ""
		if routes[index].HasHead && !routes[index].NotFoundPage {
			head = "export { Head } from " + strconv.Quote(filepath.Join(root, filepath.FromSlash(routes[index].View))) + ";\n"
		}
		source := imports.String() + head + "export function Page(props: Record<string, unknown>) {\n  return " + body + ";\n}\n"
		name := fmt.Sprintf("page-%d.tsx", index)
		if err := os.WriteFile(filepath.Join(views, name), []byte(source), 0o644); err != nil {
			return err
		}
		routes[index].View = ".bifrost/views/" + name
	}
	return nil
}

var headExportPattern = regexp.MustCompile(`(?m)^\s*export\s+(?:function|const|let|var)\s+Head\b`)

func hasHeadExport(filePath string) bool {
	data, err := os.ReadFile(filePath)
	return err == nil && headExportPattern.Match(data)
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

func conventionPattern(relative string) (string, error) {
	if relative == "." {
		return "/{$}", nil
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	for index, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "{}?#%") {
			return "", fmt.Errorf("invalid segment %q", part)
		}
		if strings.HasSuffix(part, "_") {
			name := strings.TrimSuffix(part, "_")
			if name == "" || !goIdentifier(name) {
				return "", fmt.Errorf("invalid dynamic segment %q", part)
			}
			parts[index] = "{" + name + "}"
		}
	}
	return "/" + strings.Join(parts, "/"), nil
}

func goIdentifier(value string) bool {
	for index, r := range value {
		if r != '_' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (index == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return value != ""
}

func conventionModule(ctx context.Context, root string, routes []conventionRoute, hasGo bool) (string, string, error) {
	for _, route := range routes {
		hasGo = hasGo || route.PageGo
	}
	if !hasGo {
		return "", "", nil
	}
	command := exec.CommandContext(ctx, "go", "env", "GOMOD")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		return "", "", err
	}
	modFile := strings.TrimSpace(string(output))
	if modFile == "" || modFile == os.DevNull {
		return "", "", errors.New("bifrost: Go convention files require an applicable go.mod")
	}
	moduleDir := filepath.Dir(modFile)
	data, err := os.ReadFile(modFile)
	if err != nil {
		return "", "", err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 || fields[0] != "module" {
		return "", "", fmt.Errorf("bifrost: cannot read module path from %s", modFile)
	}
	relativeRoot, err := filepath.Rel(moduleDir, root)
	if err != nil || relativeRoot == ".." || strings.HasPrefix(relativeRoot, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("bifrost: app root is outside module %s", moduleDir)
	}
	modulePath := fields[1]
	if relativeRoot != "." {
		modulePath += "/" + filepath.ToSlash(relativeRoot)
	}
	return modulePath, moduleDir, nil
}

func conventionAlias(aliases map[string]string, importPath string) string {
	if alias := aliases[importPath]; alias != "" {
		return alias
	}
	alias := fmt.Sprintf("route%d", len(aliases))
	aliases[importPath] = alias
	return alias
}

func hasSymbol(ctx context.Context, dir, importPath, name string) bool {
	command := exec.CommandContext(ctx, "go", "doc", importPath+"."+name)
	command.Dir = dir
	return command.Run() == nil
}

func directoryContains(parent, child string) bool {
	return parent == "." || parent == child || strings.HasPrefix(child, parent+"/")
}

func writeConventionMain(root, generated string, routes []conventionRoute, goDirs []conventionGoDir) error {
	importsByPath := make(map[string]string)
	var declarations strings.Builder
	var loaders strings.Builder
	hasNotFoundPage := false
	for index, route := range routes {
		loader := "nil"
		if route.NotFoundPage {
			loader = "loadNotFound"
			hasNotFoundPage = true
		}
		if route.HasLoader {
			importsByPath[route.ImportPath] = route.Alias
			loader = route.Alias + ".Load"
			if len(route.ErrorViews) > 0 || route.NotFoundView != "" {
				loader = fmt.Sprintf("load%d", index)
				fmt.Fprintf(&loaders, "func %s(r *http.Request) (any, error) {\n\tprops, err := %s.Load(r)\n\tif err == nil {\n", loader, route.Alias)
				if len(route.ErrorViews) > 0 {
					fmt.Fprintf(&loaders, "\t\tif data, ok := props.(bifrost.PageData); ok {\n\t\t\tdata.ErrorFallbacks = %d\n\t\t\treturn data, nil\n\t\t}\n\t\treturn bifrost.PageData{Props: props, ErrorFallbacks: %d}, nil\n", len(route.ErrorViews), len(route.ErrorViews))
				} else {
					fmt.Fprintf(&loaders, "\t\treturn props, nil\n")
				}
				fmt.Fprintf(&loaders, "\t}\n\tif bifrost.IsRedirect(err) {\n\t\treturn nil, err\n\t}\n\tstatus, ok := bifrost.ErrorStatus(err)\n\tif !ok {\n\t\tstatus = http.StatusInternalServerError\n\t}\n")
				if route.NotFoundView != "" {
					fmt.Fprintf(&loaders, "\tif status == http.StatusNotFound {\n\t\treturn bifrost.PageData{Props: map[string]any{\"__bifrostNotFound\": true}, Status: status, ErrorFallbacks: %d}, nil\n\t}\n", len(route.ErrorViews))
				}
				if len(route.ErrorViews) > 0 {
					fmt.Fprintf(&loaders, "\tmessage := http.StatusText(status)\n\tif os.Getenv(\"BIFROST_DEV_DIR\") != \"\" {\n\t\tmessage = err.Error()\n\t}\n\treturn bifrost.PageData{Props: map[string]any{\"__bifrostError\": message, \"__bifrostErrorLevel\": %d}, Status: status, ErrorFallbacks: %d}, nil\n", len(route.ErrorViews)-1, len(route.ErrorViews))
				} else {
					fmt.Fprintf(&loaders, "\treturn nil, err\n")
				}
				fmt.Fprintf(&loaders, "}\n\n")
			}
		} else if len(route.ErrorViews) > 0 {
			loader = fmt.Sprintf("load%d", index)
			fmt.Fprintf(&loaders, "func %s(*http.Request) (any, error) {\n\treturn bifrost.PageData{ErrorFallbacks: %d}, nil\n}\n\n", loader, len(route.ErrorViews))
		}
		fmt.Fprintf(&declarations, "\t\t\tbifrost.Server(%s, %s, %s),\n", strconv.Quote(route.Pattern), strconv.Quote(route.View), loader)
	}
	if hasNotFoundPage {
		fmt.Fprintf(&loaders, "func loadNotFound(*http.Request) (any, error) {\n\treturn bifrost.PageData{Status: http.StatusNotFound}, nil\n}\n\n")
	}
	for _, directory := range goDirs {
		if directory.Middleware || directory.Serve || len(directory.HTTPMethods) > 0 {
			importsByPath[directory.ImportPath] = directory.Alias
		}
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
	for _, route := range routes {
		handler := "http.Handler(pageMux)"
		for index := len(goDirs) - 1; index >= 0; index-- {
			if goDirs[index].Middleware && directoryContains(goDirs[index].Directory, route.Directory) {
				handler = goDirs[index].Alias + ".Middleware(" + handler + ")"
			}
		}
		fmt.Fprintf(&registrations, "\tmux.Handle(%s, %s)\n", strconv.Quote("GET "+route.Pattern), handler)
	}
	for _, directory := range goDirs {
		for _, method := range directory.HTTPMethods {
			handler := "http.HandlerFunc(" + directory.Alias + "." + method + ")"
			for index := len(goDirs) - 1; index >= 0; index-- {
				if goDirs[index].Middleware && directoryContains(goDirs[index].Directory, directory.Directory) {
					handler = goDirs[index].Alias + ".Middleware(" + handler + ")"
				}
			}
			fmt.Fprintf(&registrations, "\tmux.Handle(%s, %s)\n", strconv.Quote(strings.ToUpper(method)+" "+directory.Pattern), handler)
		}
	}
	serve := "return serve(ctx, mux)"
	for _, directory := range goDirs {
		if directory.Serve {
			serve = "return " + directory.Alias + ".Serve(ctx, mux)"
			break
		}
	}
	source := "package main\n\nimport (\n\t\"context\"\n\t\"embed\"\n\t\"errors\"\n\t\"flag\"\n\t\"io/fs\"\n\t\"log\"\n\t\"net/http\"\n\t\"os\"\n\t\"os/signal\"\n\t\"syscall\"\n\t\"time\"\n\n\t\"github.com/3-lines-studio/bifrost\"\n" + imports.String() + ")\n\n//go:embed all:build\nvar embedded embed.FS\n\n" + loaders.String() + "func main() {\n\tif err := run(); err != nil {\n\t\tlog.Fatal(err)\n\t}\n}\n\nfunc run() error {\n\tassets, err := fs.Sub(embedded, \"build\")\n\tif err != nil {\n\t\treturn err\n\t}\n\tapp, err := bifrost.New(bifrost.Config{SourceRoot: " + strconv.Quote(root) + ", Assets: assets, Routes: []bifrost.Route{\n" + declarations.String() + "\t}})\n\tif err != nil {\n\t\treturn err\n\t}\n\tif bifrost.Building() {\n\t\treturn nil\n\t}\n\tctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)\n\tdefer stop()\n\tdefer func() {\n\t\tcloseCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)\n\t\tdefer cancel()\n\t\t_ = app.Close(closeCtx)\n\t}()\n\tpageMux := http.NewServeMux()\n\tif err := app.Register(pageMux); err != nil {\n\t\treturn err\n\t}\n\tmux := http.NewServeMux()\n" + registrations.String() + "\tmux.Handle(\"/\", pageMux)\n\t" + serve + "\n}\n\nfunc serve(ctx context.Context, handler http.Handler) error {\n\taddr := os.Getenv(\"BIFROST_ADDR\")\n\tif addr == \"\" {\n\t\taddr = \":8080\"\n\t}\n\tflag.StringVar(&addr, \"addr\", addr, \"HTTP listen address\")\n\tflag.Parse()\n\tserver := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10*time.Second}\n\tdone := make(chan error, 1)\n\tgo func() { done <- server.ListenAndServe() }()\n\tselect {\n\tcase err := <-done:\n\t\tif errors.Is(err, http.ErrServerClosed) {\n\t\t\treturn nil\n\t\t}\n\t\treturn err\n\tcase <-ctx.Done():\n\t}\n\tshutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)\n\tdefer cancel()\n\tif err := server.Shutdown(shutdownCtx); err != nil {\n\t\t_ = server.Close()\n\t\treturn err\n\t}\n\terr := <-done\n\tif errors.Is(err, http.ErrServerClosed) {\n\t\treturn nil\n\t}\n\treturn err\n}\n"
	formatted, err := format.Source([]byte(source))
	if err != nil {
		return err
	}
	path := filepath.Join(generated, "main.go")
	current, _ := os.ReadFile(path)
	if bytes.Equal(current, formatted) {
		return nil
	}
	return os.WriteFile(path, formatted, 0o644)
}

func writeConventionModule(generated, userModuleDir string) error {
	version := bifrost.Version
	bifrostModuleDir := ""
	if !strings.HasPrefix(version, "v") || strings.HasPrefix(version, "v0.0.0-") {
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

	if userModuleDir != "" {
		data, err := os.ReadFile(filepath.Join(userModuleDir, "go.mod"))
		if err != nil {
			return err
		}
		fields := strings.Fields(string(data))
		if len(fields) < 2 || fields[0] != "module" {
			return fmt.Errorf("bifrost: cannot read module path from %s", filepath.Join(userModuleDir, "go.mod"))
		}
		userModulePath := fields[1]
		if userModulePath == "github.com/3-lines-studio/bifrost" {
			bifrostModuleDir = userModuleDir
		} else {
			module.WriteString("require ")
			module.WriteString(userModulePath)
			module.WriteString(" v0.0.0\n\nreplace ")
			module.WriteString(userModulePath)
			module.WriteString(" => ")
			module.WriteString(strconv.Quote(filepath.ToSlash(userModuleDir)))
			module.WriteString("\n")
		}
	}
	if bifrostModuleDir != "" {
		module.WriteString("\nreplace github.com/3-lines-studio/bifrost => ")
		module.WriteString(strconv.Quote(filepath.ToSlash(bifrostModuleDir)))
		module.WriteString("\n")
	}
	return os.WriteFile(filepath.Join(generated, "go.mod"), []byte(module.String()), 0o644)
}

func buildConvention(ctx context.Context, app conventionApp, options builder.Options, buildExecutable bool) error {
	options.Package = app.Package
	options.Dir = app.WorkDir
	options.Output = app.Output
	if err := builder.Build(ctx, options); err != nil {
		return err
	}
	if !buildExecutable {
		return nil
	}
	command := exec.CommandContext(ctx, "go", "build", "-o", app.Executable, app.Package)
	command.Dir = app.WorkDir
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}
