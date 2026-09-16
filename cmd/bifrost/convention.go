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

type conventionParam struct {
	Name     string
	Value    string
	Segments bool
}

type conventionMetadata struct {
	View     string
	Generate bool
}

type conventionWrapper struct {
	Path     string
	Template bool
}

type conventionRoute struct {
	Directory    string
	Pattern      string
	Params       []conventionParam
	View         string
	ImportPath   string
	Alias        string
	PageGo       bool
	HasLoader    bool
	HasHead      bool
	ErrorViews   []string
	NotFoundView string
	LoadingView  string
	NotFoundPage bool
}

type conventionGoDir struct {
	Directory   string
	Pattern     string
	Params      []conventionParam
	ImportPath  string
	Alias       string
	Middleware  bool
	Serve       bool
	HTTPMethods []string
}

func conventionRoots(dir, packagePath string) (string, string, bool, error) {
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
	if hasConventionPage(filepath.Join(projectRoot, "app")) || hasConventionGo(filepath.Join(projectRoot, "app")) || hasConventionMarker(filepath.Join(projectRoot, "app")) {
		return projectRoot, filepath.Join(projectRoot, "app"), true, nil
	}
	if hasConventionPage(projectRoot) || hasConventionMarker(projectRoot) {
		return projectRoot, projectRoot, true, nil
	}
	return projectRoot, "", false, nil
}

func conventionHint(projectRoot string, err error) error {
	if hasConventionPage(projectRoot) || hasConventionMarker(projectRoot) || !hasConventionGo(projectRoot) {
		return err
	}
	return fmt.Errorf("%w\nbifrost: %s has route.go but no page.tsx and no app directory; a convention app needs one of them", err, filepath.ToSlash(projectRoot))
}

func conventionWalk(root string, match func(path string, entry os.DirEntry) bool) bool {
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

func hasConventionPage(root string) bool {
	return conventionWalk(root, func(_ string, entry os.DirEntry) bool {
		return !entry.IsDir() && entry.Name() == "page.tsx"
	})
}

func hasConventionGo(root string) bool {
	return conventionWalk(root, func(_ string, entry os.DirEntry) bool {
		return !entry.IsDir() && conventionGoFile(entry.Name())
	})
}

func hasConventionMarker(root string) bool {
	return conventionWalk(root, func(_ string, entry os.DirEntry) bool {
		return entry.IsDir() && conventionMarkerName(entry.Name())
	})
}

func conventionGoFile(name string) bool {
	return name == "route.go" || name == "middleware.go" || name == "server.go"
}

func conventionMarkerName(name string) bool {
	if strings.HasPrefix(name, "_") {
		return true
	}
	if strings.HasSuffix(name, "~") {
		return true
	}
	return strings.TrimRight(name, "_") != name
}

type conventionApp struct {
	ProjectRoot string
	WorkDir     string
	Package     string
	Output      string
	Executable  string
	RouteParams map[string][]string
}

func prepareConventionApp(ctx context.Context, projectRoot, routeRoot string) (conventionApp, error) {
	routes, err := discoverConventionRoutes(routeRoot)
	if err != nil {
		return conventionApp{}, err
	}
	goDirs, err := discoverConventionGo(routeRoot)
	if err != nil {
		return conventionApp{}, err
	}
	if len(routes) == 0 && len(goDirs) == 0 {
		return conventionApp{}, fmt.Errorf("bifrost: no page.tsx or route.go found under %s", routeRoot)
	}
	routes, err = appendNotFoundRoutes(routeRoot, routes)
	if err != nil {
		return conventionApp{}, err
	}
	modulePath, moduleDir, err := conventionModule(ctx, routeRoot, routes, len(goDirs) > 0)
	if err != nil {
		return conventionApp{}, err
	}
	generated := filepath.Join(projectRoot, ".bifrost", "app")
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
	if err := writeConventionViews(projectRoot, routeRoot, routes); err != nil {
		return conventionApp{}, err
	}
	if err := writeConventionMain(projectRoot, generated, routes, goDirs); err != nil {
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
	return conventionApp{ProjectRoot: projectRoot, WorkDir: generated, Package: ".", Output: filepath.Join(generated, "build"), Executable: filepath.Join(projectRoot, ".bifrost", "bifrost-app"), RouteParams: conventionRouteParams(routes)}, nil
}

func conventionRouteParams(routes []conventionRoute) map[string][]string {
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

func discoverConventionRoutes(root string) ([]conventionRoute, error) {
	var routes []conventionRoute
	patterns := make(map[string]string)
	err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if skip, err := skipConventionDirectory(filePath, entry); err != nil {
			return err
		} else if skip {
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
		pattern, params, err := conventionPattern(relative)
		if err != nil {
			return fmt.Errorf("bifrost: route %s: %w", filepath.ToSlash(relative), err)
		}
		if previous := patterns[pattern]; previous != "" {
			return fmt.Errorf("bifrost: duplicate route %q from %s and %s", pattern, previous, filepath.ToSlash(relative))
		}
		patterns[pattern] = filepath.ToSlash(relative)
		route := conventionRoute{Directory: filepath.ToSlash(relative), Pattern: pattern, Params: params, View: filepath.ToSlash(strings.TrimPrefix(filePath, root+string(filepath.Separator))), HasHead: hasHeadExport(filePath)}
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
		if skip, err := skipConventionDirectory(filePath, entry); err != nil {
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
		base, _, err := conventionPattern(relative)
		if err != nil {
			return err
		}
		pattern := strings.TrimSuffix(base, "/{$}") + "/{path...}"
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
		if skip, err := skipConventionDirectory(filePath, entry); err != nil {
			return err
		} else if skip {
			return filepath.SkipDir
		}
		if entry.IsDir() || !conventionGoFile(entry.Name()) {
			return nil
		}
		relative, err := filepath.Rel(root, filepath.Dir(filePath))
		if err != nil {
			return err
		}
		pattern, params, err := conventionPattern(relative)
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
	items := make([]conventionGoDir, 0, len(byDirectory))
	for _, item := range byDirectory {
		items = append(items, item)
	}
	slices.SortFunc(items, func(a, b conventionGoDir) int { return strings.Compare(a.Directory, b.Directory) })
	return items, nil
}

func writeConventionViews(projectRoot, routeRoot string, routes []conventionRoute) error {
	views := filepath.Join(projectRoot, ".bifrost", "views")
	if err := os.RemoveAll(views); err != nil {
		return err
	}
	if err := os.MkdirAll(views, 0o755); err != nil {
		return err
	}
	needsMetadata := false
	for index := range routes {
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
		metadata, err := conventionMetadataSources(append(slices.Clone(layouts), pageView))
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
		if err := writeConventionImport(&imports, filepath.Join(routeRoot, filepath.FromSlash(routes[index].View)), export, "RoutePage"); err != nil {
			return err
		}
		for wrapperIndex, wrapper := range wrappers {
			name := "Layout"
			if wrapper.Template {
				name = "Template"
			}
			if err := writeConventionImport(&imports, wrapper.Path, name, fmt.Sprintf("%s%d", name, wrapperIndex)); err != nil {
				return err
			}
		}
		for errorIndex, errorView := range routes[index].ErrorViews {
			if err := writeConventionImport(&imports, errorView, "Error", fmt.Sprintf("ErrorPage%d", errorIndex)); err != nil {
				return err
			}
		}
		if routes[index].NotFoundView != "" {
			if err := writeConventionImport(&imports, routes[index].NotFoundView, "NotFound", "NotFound"); err != nil {
				return err
			}
		}
		if routes[index].LoadingView != "" {
			if err := writeConventionImport(&imports, routes[index].LoadingView, "Loading", "RouteLoading"); err != nil {
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
		case routes[index].HasHead && !routes[index].NotFoundPage:
			head = "export { Head } from " + strconv.Quote(pageView) + ";\n"
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
		if err := os.WriteFile(filepath.Join(views, "metadata.tsx"), []byte(conventionMetadataSource), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func metadataValues(sources []conventionMetadata) string {
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

func metadataUsesProps(sources []conventionMetadata) bool {
	return slices.ContainsFunc(sources, func(source conventionMetadata) bool { return source.Generate })
}

var headExportPattern = regexp.MustCompile(`(?m)^\s*export\s+(?:function|const|let|var)\s+Head\b`)

var defaultExportPattern = regexp.MustCompile(`(?m)^\s*export\s+default\b`)

var metadataExportPattern = regexp.MustCompile(`(?m)^\s*export\s+(?:const|let|var)\s+metadata\b`)

var generateMetadataExportPattern = regexp.MustCompile(`(?m)^\s*export\s+(?:async\s+)?(?:function|const|let|var)\s+generateMetadata\b`)

const conventionMetadataSource = `import type { ReactNode } from 'react';

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

func writeConventionImport(imports *strings.Builder, filePath, name, local string) error {
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

func hasMetadataExport(filePath string) bool {
	data, err := os.ReadFile(filePath)
	return err == nil && metadataExportPattern.Match(data)
}

func hasGenerateMetadataExport(filePath string) bool {
	data, err := os.ReadFile(filePath)
	return err == nil && generateMetadataExportPattern.Match(data)
}

func conventionMetadataSources(views []string) ([]conventionMetadata, error) {
	sources := make([]conventionMetadata, 0, len(views))
	for _, view := range views {
		static, generated := hasMetadataExport(view), hasGenerateMetadataExport(view)
		if static && generated {
			return nil, fmt.Errorf("bifrost: %s exports both metadata and generateMetadata; keep one", filepath.ToSlash(view))
		}
		if static || generated {
			sources = append(sources, conventionMetadata{View: view, Generate: generated})
		}
	}
	return sources, nil
}

func inheritedWrappers(root, directory string) []conventionWrapper {
	var wrappers []conventionWrapper
	appendDirectory := func(current string) {
		for _, name := range []string{"layout.tsx", "template.tsx"} {
			path := filepath.Join(root, current, name)
			if fileExists(path) {
				wrappers = append(wrappers, conventionWrapper{Path: path, Template: name == "template.tsx"})
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

func skipConventionDirectory(filePath string, entry os.DirEntry) (bool, error) {
	if !entry.IsDir() {
		return false, nil
	}
	if entry.Name() == ".bifrost" {
		return true, nil
	}
	if strings.HasPrefix(entry.Name(), "_") {
		return true, conventionPrivateDirectory(filePath)
	}
	return false, nil
}

func conventionPrivateDirectory(directory string) error {
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

func conventionPattern(relative string) (string, []conventionParam, error) {
	if relative == "." {
		return "/{$}", nil, nil
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	segments := make([]string, 0, len(parts))
	var params []conventionParam
	for index, part := range parts {
		segment, param, err := conventionSegment(part, index == len(parts)-1)
		if err != nil {
			return "", nil, err
		}
		if param == nil {
			if segment != "" {
				segments = append(segments, segment)
			}
			continue
		}
		if err := appendConventionParam(&params, *param); err != nil {
			return "", nil, err
		}
		segments = append(segments, segment)
	}
	if len(segments) == 0 {
		return "/{$}", params, nil
	}
	return "/" + strings.Join(segments, "/"), params, nil
}

func conventionSegment(part string, last bool) (string, *conventionParam, error) {
	if err := conventionSegmentName(part); err != nil {
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
	value := conventionPathValue(name)
	suffix := ""
	if trailing == 2 {
		suffix = "..."
	}
	return "{" + value + suffix + "}", &conventionParam{Name: name, Value: value, Segments: trailing == 2}, nil
}

func appendConventionParam(params *[]conventionParam, param conventionParam) error {
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

func conventionSegmentName(part string) error {
	if part == "" || part == "." || part == ".." {
		return fmt.Errorf("invalid segment %q", part)
	}
	for _, r := range part {
		if goIdentifierRune(r) || r == '-' || r == '.' || r == '+' || r == '~' {
			continue
		}
		switch r {
		case '[', ']':
			return fmt.Errorf("%s: brackets are not valid; name the folder %s", part, conventionParameterFolder(part))
		case '(', ')':
			return fmt.Errorf("%s: parentheses are not valid; name the folder %s~", part, strings.Trim(part, "()"))
		case '@':
			return fmt.Errorf("%s: parallel route folders are not supported", part)
		}
		return fmt.Errorf("invalid character %q in segment %s", r, part)
	}
	return nil
}

func conventionParameterFolder(part string) string {
	name := strings.TrimRight(strings.TrimLeft(part, "["), "]")
	if rest, ok := strings.CutPrefix(name, "..."); ok {
		return rest + "__"
	}
	return name + "_"
}

func conventionPathValue(name string) string {
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

func conventionStandardImports(segments bool) string {
	imports := "\t\"context\"\n\t\"embed\"\n\t\"errors\"\n\t\"flag\"\n\t\"io/fs\"\n\t\"log\"\n\t\"net/http\"\n\t\"os\"\n\t\"os/signal\"\n\t\"reflect\"\n"
	if segments {
		imports += "\t\"strings\"\n"
	}
	return imports + "\t\"syscall\"\n\t\"time\"\n"
}

func conventionParams(params []conventionParam) string {
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

func writeConventionRequestProps(loaders *strings.Builder, segments bool) {
	loaders.WriteString("func requestPage(r *http.Request, params map[string]any, props any, fallbacks int) (any, error) {\n\tdata, ok := props.(bifrost.PageData)\n\tif !ok {\n\t\tdata = bifrost.PageData{Props: props}\n\t}\n\tvalues, err := requestProps(data.Props)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\tvalues[\"params\"] = params\n\tvalues[\"searchParams\"] = requestSearchParams(r)\n\tvalues[\"pathname\"] = r.URL.EscapedPath()\n\tdata.Props = values\n\tdata.ErrorFallbacks = fallbacks\n\treturn data, nil\n}\n\n")
	loaders.WriteString("func requestProps(props any) (map[string]any, error) {\n\tif props == nil {\n\t\treturn map[string]any{}, nil\n\t}\n\tvalue := reflect.ValueOf(props)\n\tif value.Kind() != reflect.Map || value.Type().Key().Kind() != reflect.String {\n\t\treturn nil, errors.New(\"bifrost: a page loader must return a map with string keys, or bifrost.PageData with one\")\n\t}\n\tvalues := make(map[string]any, value.Len())\n\titer := value.MapRange()\n\tfor iter.Next() {\n\t\tvalues[iter.Key().String()] = iter.Value().Interface()\n\t}\n\treturn values, nil\n}\n\n")
	loaders.WriteString("func requestSearchParams(r *http.Request) map[string]any {\n\tquery := r.URL.Query()\n\tvalues := make(map[string]any, len(query))\n\tfor key, items := range query {\n\t\tif len(items) == 1 {\n\t\t\tvalues[key] = items[0]\n\t\t\tcontinue\n\t\t}\n\t\tvalues[key] = items\n\t}\n\treturn values\n}\n\n")
	if !segments {
		return
	}
	loaders.WriteString("func requestSegments(value string) []string {\n\tif value == \"\" {\n\t\treturn []string{}\n\t}\n\treturn strings.Split(value, \"/\")\n}\n\n")
}

func conventionPathValues(builder *strings.Builder, name, handler string, params []conventionParam) string {
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

func writeConventionMain(root, generated string, routes []conventionRoute, goDirs []conventionGoDir) error {
	importsByPath := make(map[string]string)
	var declarations strings.Builder
	var loaders strings.Builder
	hasNotFoundPage := false
	hasSegments := false
	for index, route := range routes {
		if route.NotFoundPage {
			hasNotFoundPage = true
			fmt.Fprintf(&declarations, "\t\t\tbifrost.Server(%s, %s, loadNotFound).WithNavigation(),\n", strconv.Quote(route.Pattern), strconv.Quote(route.View))
			continue
		}
		hasSegments = hasSegments || slices.ContainsFunc(route.Params, func(param conventionParam) bool { return param.Segments })
		loader := fmt.Sprintf("load%d", index)
		params := conventionParams(route.Params)
		if route.HasLoader {
			importsByPath[route.ImportPath] = route.Alias
			fmt.Fprintf(&loaders, "func %s(r *http.Request) (any, error) {\n\tprops, err := %s.Load(r)\n\tif err == nil {\n\t\treturn requestPage(r, %s, props, %d)\n\t}\n\tif bifrost.IsRedirect(err) {\n\t\treturn nil, err\n\t}\n", loader, route.Alias, params, len(route.ErrorViews))
			if len(route.ErrorViews) == 0 && route.NotFoundView == "" {
				fmt.Fprintf(&loaders, "\treturn nil, err\n")
			} else {
				fmt.Fprintf(&loaders, "\tstatus, ok := bifrost.ErrorStatus(err)\n\tif !ok {\n\t\tstatus = http.StatusInternalServerError\n\t}\n")
				if route.NotFoundView != "" {
					fmt.Fprintf(&loaders, "\tif status == http.StatusNotFound {\n\t\treturn bifrost.PageData{Props: map[string]any{\"__bifrostNotFound\": true}, Status: status, ErrorFallbacks: %d}, nil\n\t}\n", len(route.ErrorViews))
				}
				if len(route.ErrorViews) > 0 {
					fmt.Fprintf(&loaders, "\tmessage := http.StatusText(status)\n\tif os.Getenv(\"BIFROST_DEV_DIR\") != \"\" {\n\t\tmessage = err.Error()\n\t}\n\treturn bifrost.PageData{Props: map[string]any{\"__bifrostError\": message, \"__bifrostErrorLevel\": %d}, Status: status, ErrorFallbacks: %d}, nil\n", len(route.ErrorViews)-1, len(route.ErrorViews))
				} else {
					fmt.Fprintf(&loaders, "\treturn nil, err\n")
				}
			}
			fmt.Fprintf(&loaders, "}\n\n")
		} else {
			fmt.Fprintf(&loaders, "func %s(r *http.Request) (any, error) {\n\treturn requestPage(r, %s, nil, %d)\n}\n\n", loader, params, len(route.ErrorViews))
		}
		fmt.Fprintf(&declarations, "\t\t\tbifrost.Server(%s, %s, %s).WithNavigation(),\n", strconv.Quote(route.Pattern), strconv.Quote(route.View), loader)
	}
	if hasNotFoundPage {
		fmt.Fprintf(&loaders, "func loadNotFound(r *http.Request) (any, error) {\n\treturn bifrost.PageData{Props: map[string]any{\"pathname\": r.URL.EscapedPath()}, Status: http.StatusNotFound}, nil\n}\n\n")
	}
	writeConventionRequestProps(&loaders, hasSegments)
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
	var pathValues strings.Builder
	for index, route := range routes {
		handler := "http.Handler(pageMux)"
		for index := len(goDirs) - 1; index >= 0; index-- {
			if goDirs[index].Middleware && directoryContains(goDirs[index].Directory, route.Directory) {
				handler = goDirs[index].Alias + ".Middleware(" + handler + ")"
			}
		}
		handler = conventionPathValues(&pathValues, fmt.Sprintf("pathValues%d", index), handler, route.Params)
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
			handler = conventionPathValues(&pathValues, fmt.Sprintf("pathValuesGo%d", directoryIndex), handler, directory.Params)
			fmt.Fprintf(&registrations, "\tmux.Handle(%s, %s)\n", strconv.Quote(strings.ToUpper(method)+" "+directory.Pattern), handler)
		}
	}
	serve := "return serve(ctx, app.ResolveMarkdown(mux))"
	for _, directory := range goDirs {
		if directory.Serve {
			serve = "return " + directory.Alias + ".Serve(ctx, app.ResolveMarkdown(mux))"
			break
		}
	}
	source := "package main\n\nimport (\n" + conventionStandardImports(hasSegments) + "\n\t\"github.com/3-lines-studio/bifrost\"\n" + imports.String() + ")\n\n//go:embed all:build\nvar embedded embed.FS\n\n" + loaders.String() + pathValues.String() + "func main() {\n\tif err := run(); err != nil {\n\t\tlog.Fatal(err)\n\t}\n}\n\nfunc run() error {\n\tassets, err := fs.Sub(embedded, \"build\")\n\tif err != nil {\n\t\treturn err\n\t}\n\tapp, err := bifrost.New(bifrost.Config{SourceRoot: " + strconv.Quote(root) + ", Assets: assets, Routes: []bifrost.Route{\n" + declarations.String() + "\t}})\n\tif err != nil {\n\t\treturn err\n\t}\n\tif bifrost.Building() {\n\t\treturn nil\n\t}\n\tctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)\n\tdefer stop()\n\tdefer func() {\n\t\tcloseCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)\n\t\tdefer cancel()\n\t\t_ = app.Close(closeCtx)\n\t}()\n\tpageMux := http.NewServeMux()\n\tif err := app.Register(pageMux); err != nil {\n\t\treturn err\n\t}\n\tmux := http.NewServeMux()\n" + registrations.String() + "\tmux.Handle(\"/\", pageMux)\n\t" + serve + "\n}\n\nfunc serve(ctx context.Context, handler http.Handler) error {\n\taddr := os.Getenv(\"BIFROST_ADDR\")\n\tif addr == \"\" {\n\t\taddr = \":8080\"\n\t}\n\tflag.StringVar(&addr, \"addr\", addr, \"HTTP listen address\")\n\tflag.Parse()\n\tserver := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10*time.Second}\n\tdone := make(chan error, 1)\n\tgo func() { done <- server.ListenAndServe() }()\n\tselect {\n\tcase err := <-done:\n\t\tif errors.Is(err, http.ErrServerClosed) {\n\t\t\treturn nil\n\t\t}\n\t\treturn err\n\tcase <-ctx.Done():\n\t}\n\tshutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)\n\tdefer cancel()\n\tif err := server.Shutdown(shutdownCtx); err != nil {\n\t\t_ = server.Close()\n\t\treturn err\n\t}\n\terr := <-done\n\tif errors.Is(err, http.ErrServerClosed) {\n\t\treturn nil\n\t}\n\treturn err\n}\n"
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
	options.RouteParams = app.RouteParams
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
