package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost"
)

func TestDiscoverAppRouterRoutes(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"page.tsx", "about/page.tsx", "posts/slug_/page.tsx"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	routes, err := discoverRoutes(root)
	if err != nil {
		t.Fatal(err)
	}
	var patterns []string
	for _, route := range routes {
		patterns = append(patterns, route.Pattern)
	}
	if got := strings.Join(patterns, ","); got != "/about,/posts/{slug},/{$}" {
		t.Fatalf("patterns = %q", got)
	}
}

func TestAppRouterRoots(t *testing.T) {
	projectRoot := t.TempDir()
	appRoot := filepath.Join(projectRoot, "app")
	if err := os.Mkdir(appRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appRoot, "page.tsx"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	project, routes, ok, err := appRouterRoots(".", projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || project != projectRoot || routes != appRoot {
		t.Fatalf("roots = %q, %q, %t", project, routes, ok)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "page.tsx"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := appRouterRoots(".", projectRoot); err == nil {
		t.Fatal("ambiguous route roots were accepted")
	}
}

func TestAppRouterRootsFollowNestedPages(t *testing.T) {
	projectRoot := t.TempDir()
	postsRoot := filepath.Join(projectRoot, "posts")
	if err := os.Mkdir(postsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(postsRoot, "page.tsx"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	project, routes, ok, err := appRouterRoots(".", projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || project != projectRoot || routes != projectRoot {
		t.Fatalf("roots without a root page = %q, %q, %t", project, routes, ok)
	}
	if err := os.Remove(filepath.Join(postsRoot, "page.tsx")); err != nil {
		t.Fatal(err)
	}
	appRoot := filepath.Join(projectRoot, "app", "posts")
	if err := os.MkdirAll(appRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appRoot, "page.tsx"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	project, routes, ok, err = appRouterRoots(".", projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || project != projectRoot || routes != filepath.Join(projectRoot, "app") {
		t.Fatalf("roots with an app directory = %q, %q, %t", project, routes, ok)
	}
}

func TestNestedAppRouterViewUsesProjectRelativePath(t *testing.T) {
	projectRoot := t.TempDir()
	routeRoot := filepath.Join(projectRoot, "app")
	if err := os.Mkdir(routeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(routeRoot, "page.tsx"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	routes, err := discoverRoutes(routeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeViews(projectRoot, routeRoot, routes); err != nil {
		t.Fatal(err)
	}
	if routes[0].View != ".bifrost/views/page-0.tsx" {
		t.Fatalf("view = %q", routes[0].View)
	}
	source, err := os.ReadFile(filepath.Join(projectRoot, routes[0].View))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), filepath.Join(routeRoot, "page.tsx")) {
		t.Fatalf("wrapper = %s", source)
	}
}

func TestAppRouterPatternRejectsInvalidDynamicSegment(t *testing.T) {
	if _, _, err := routePattern("posts/_"); err == nil {
		t.Fatal("empty dynamic segment was accepted")
	}
}

func TestAppRouterPatternNextSyntax(t *testing.T) {
	cases := []struct {
		relative string
		pattern  string
		params   []routeParam
	}{
		{"about", "/about", nil},
		{"posts/slug_", "/posts/{slug}", []routeParam{{Name: "slug", Value: "slug"}}},
		{"posts/post-id_", "/posts/{post_id}", []routeParam{{Name: "post-id", Value: "post_id"}}},
		{"docs/slug__", "/docs/{slug...}", []routeParam{{Name: "slug", Value: "slug", Segments: true}}},
		{"marketing~/about", "/about", nil},
		{"marketing~", "/{$}", nil},
	}
	for _, test := range cases {
		pattern, params, err := routePattern(test.relative)
		if err != nil {
			t.Fatalf("%s: %v", test.relative, err)
		}
		if pattern != test.pattern {
			t.Fatalf("%s: pattern = %q, want %q", test.relative, pattern, test.pattern)
		}
		if !slices.Equal(params, test.params) {
			t.Fatalf("%s: params = %v, want %v", test.relative, params, test.params)
		}
	}
}

func TestAppRouterPatternRejectsNextOnlySyntax(t *testing.T) {
	cases := map[string]string{
		"posts/[slug]":    "slug_",
		"posts/[...slug]": "slug__",
		"(marketing)":     "marketing~",
		"@modal":          "parallel route folders are not supported",
		"posts/slug___":   "one _ for a parameter",
		"slug__/posts":    "must be the last segment",
		"posts/post id":   "invalid character",
	}
	for relative, expected := range cases {
		_, _, err := routePattern(filepath.FromSlash(relative))
		if err == nil {
			t.Fatalf("%s: invalid segment was accepted", relative)
		}
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("%s: error = %q, want %q", relative, err, expected)
		}
	}
}

func TestAppRouterPrivateDirectoriesAreNotRouted(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"page.tsx", "_components/Card.tsx"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	routes, err := discoverRoutes(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 || routes[0].Pattern != "/{$}" {
		t.Fatalf("private directory was routed: %v", routes)
	}
	path := filepath.Join(root, "_components", "page.tsx")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := discoverRoutes(root); err == nil {
		t.Fatal("page.tsx inside a private directory was accepted")
	}
}

func TestAppRouterRouteParamsUseFolderNames(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "posts", "post-id_", "page.tsx")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("export function Page() { return null }"), 0o644); err != nil {
		t.Fatal(err)
	}
	routes, err := discoverRoutes(root)
	if err != nil {
		t.Fatal(err)
	}
	params := routeParams(routes)
	if names := params["/posts/{post_id}"]; len(names) != 1 || names[0] != "post-id" {
		t.Fatalf("params = %v", params)
	}
}

func TestAppRouterViewsRenderPendingTrees(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"page.tsx":           "export function Page() { return null }",
		"loading.tsx":        "export default function Loading() { return null }",
		"dashboard/page.tsx": "export function Page() { return null }",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	routes, err := discoverRoutes(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeViews(root, root, routes); err != nil {
		t.Fatal(err)
	}
	for index, route := range routes {
		if route.LoadingView != filepath.Join(root, "loading.tsx") {
			t.Fatalf("route %d loading view = %q", index, route.LoadingView)
		}
		view, err := os.ReadFile(filepath.Join(root, ".bifrost", "views", fmt.Sprintf("page-%d.tsx", index)))
		if err != nil {
			t.Fatal(err)
		}
		text := string(view)
		for _, expected := range []string{
			"import RouteLoading from",
			"export function renderPending(props: Record<string, unknown>, pageKey?: string) {",
			"<RouteProvider pathname={props.pathname} params={props.params} searchParams={props.searchParams}><Fragment key={pageKey}>{<RouteLoading />}</Fragment></RouteProvider>;",
		} {
			if !strings.Contains(text, expected) {
				t.Fatalf("generated view does not contain %q:\n%s", expected, text)
			}
		}
	}
}

func TestAppRouterNestedLoadingViewsWin(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"page.tsx":                    "export function Page() { return null }",
		"loading.tsx":                 "export default function Loading() { return null }",
		"dashboard/loading.tsx":       "export function Loading() { return null }",
		"dashboard/page.tsx":          "export function Page() { return null }",
		"dashboard/settings/page.tsx": "export function Page() { return null }",
		"other/page.tsx":              "export function Page() { return null }",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	routes, err := discoverRoutes(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeViews(root, root, routes); err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{
		"/{$}":                "loading.tsx",
		"/dashboard":          filepath.Join("dashboard", "loading.tsx"),
		"/dashboard/settings": filepath.Join("dashboard", "loading.tsx"),
		"/other":              "loading.tsx",
	}
	for _, route := range routes {
		if route.NotFoundPage {
			if route.LoadingView != "" {
				t.Fatalf("not found route loading view = %q", route.LoadingView)
			}
			continue
		}
		want := expected[route.Pattern]
		if want == "" {
			t.Fatalf("unexpected route %q", route.Pattern)
		}
		if filepath.ToSlash(route.LoadingView) != filepath.ToSlash(filepath.Join(root, want)) {
			t.Fatalf("route %s loading view = %q, want %q", route.Pattern, route.LoadingView, want)
		}
	}
}

func TestAppRouterViewsWrapTheTreeInTheRouteProvider(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"layout.tsx": "export function Layout({ children }) { return children }",
		"page.tsx":   "export function Page() { return null }",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	routes, err := discoverRoutes(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeViews(root, root, routes); err != nil {
		t.Fatal(err)
	}
	view, err := os.ReadFile(filepath.Join(root, ".bifrost", "views", "page-0.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(view)
	for _, expected := range []string{
		"import { RouteProvider } from 'virtual:bifrost/navigation';",
		"return <RouteProvider pathname={props.pathname} params={props.params} searchParams={props.searchParams}><Layout0 key={\"layout.tsx\"}",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("generated view does not contain %q:\n%s", expected, text)
		}
	}
}

func TestAppRouterViewsGenerateMetadataHead(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"layout.tsx":     "export const metadata = { description: 'site' }",
		"page.tsx":       "export const metadata = { title: 'home' }",
		"posts/page.tsx": "export function Page() { return null }",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	routes, err := discoverRoutes(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeViews(root, root, routes); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".bifrost", "views"))
	if err != nil {
		t.Fatal(err)
	}
	generated := make(map[string]string, len(entries))
	for _, entry := range entries {
		source, err := os.ReadFile(filepath.Join(root, ".bifrost", "views", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		generated[entry.Name()] = string(source)
	}
	if _, ok := generated["metadata.tsx"]; !ok {
		t.Fatalf("metadata runtime module was not generated: %v", generated)
	}
	var view strings.Builder
	for name, source := range generated {
		if name != "metadata.tsx" {
			view.WriteString(source)
		}
	}
	for _, expected := range []string{
		"import { metadata as Metadata0 } from " + strconv.Quote(filepath.Join(root, "layout.tsx")) + ";",
		"import { metadata as Metadata1 } from " + strconv.Quote(filepath.Join(root, "page.tsx")) + ";",
		"import { Metadata as RouteMetadata } from './metadata.tsx';",
		"export function Head() {",
		"values={[Metadata0, Metadata1]}",
	} {
		if !strings.Contains(view.String(), expected) {
			t.Fatalf("generated views do not contain %q:\n%s", expected, view.String())
		}
	}
}

func TestAppRouterViewsGenerateAsyncMetadataHead(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"layout.tsx": "export const metadata = { description: 'site' }",
		"page.tsx":   "export async function generateMetadata() { return { title: 'dynamic' } }",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	routes, err := discoverRoutes(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeViews(root, root, routes); err != nil {
		t.Fatal(err)
	}
	view, err := os.ReadFile(filepath.Join(root, ".bifrost", "views", "page-0.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(view)
	for _, expected := range []string{
		"import { generateMetadata as GenerateMetadata1 } from " + strconv.Quote(filepath.Join(root, "page.tsx")) + ";",
		"export async function renderHead(props: Record<string, unknown>) {",
		"values={[Metadata0, await GenerateMetadata1(props)]}",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("generated view does not contain %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "export function Head()") {
		t.Fatalf("generated view should not export a sync Head:\n%s", text)
	}
}

func TestAppRouterRejectsConflictingMetadata(t *testing.T) {
	cases := map[string]struct {
		page string
		want string
	}{
		"head and metadata":             {page: "export function Head() { return null }\nexport const metadata = { title: 'x' }", want: "exports both Head and metadata"},
		"metadata and generateMetadata": {page: "export const metadata = { title: 'x' }\nexport function generateMetadata() { return { title: 'y' } }", want: "exports both metadata and generateMetadata"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "page.tsx"), []byte(testCase.page), 0o644); err != nil {
				t.Fatal(err)
			}
			routes, err := discoverRoutes(root)
			if err != nil {
				t.Fatal(err)
			}
			err = writeViews(root, root, routes)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("writeViews error = %v, want %q", err, testCase.want)
			}
		})
	}
}

func TestAppRouterRejectsHeadOutsidePage(t *testing.T) {
	for _, name := range []string{"layout.tsx", "template.tsx", "error.tsx", "not-found.tsx", "loading.tsx"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			files := map[string]string{
				"page.tsx": "export function Page() { return null }",
				name:       "export function Head() { return null }",
			}
			for file, content := range files {
				if err := os.WriteFile(filepath.Join(root, file), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := discoverRoutes(root); err == nil || !strings.Contains(err.Error(), name) || !strings.Contains(err.Error(), "keep it in page.tsx") {
				t.Fatalf("discoverRoutes error = %v", err)
			}
		})
	}
}

func TestAppRouterAcceptsHeadInPageAndComponents(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"page.tsx":             "export function Head() { return null }\nexport function Page() { return null }",
		"layout.tsx":           "export function Layout({ children }) { return children }",
		"_components/card.tsx": "export function Head() { return null }",
	}
	for file, content := range files {
		path := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	routes, err := discoverRoutes(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 || !routes[0].HasHead {
		t.Fatalf("routes = %#v", routes)
	}
}

func TestAppRouterErrorViewsReceiveAnErrorAndReset(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"page.tsx":  "export function Page() { return null }",
		"error.tsx": "export function Error() { return null }",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	routes, err := discoverRoutes(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeViews(root, root, routes); err != nil {
		t.Fatal(err)
	}
	view, err := os.ReadFile(filepath.Join(root, ".bifrost", "views", "page-0.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(view)
	for _, expected := range []string{
		"import { refresh } from 'virtual:bifrost/navigation';",
		"error={new Error(String(props.__bifrostError))} reset={() => void refresh()}",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("generated view does not contain %q:\n%s", expected, text)
		}
	}
}

func TestAppRouterTemplatesNestInsideTheirLayouts(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"layout.tsx":             "export function Layout({ children }) { return children }",
		"template.tsx":           "export default function Template({ children }) { return <div>{children}</div> }",
		"dashboard/layout.tsx":   "export function Layout({ children }) { return children }",
		"dashboard/template.tsx": "export function Template({ children }) { return <div>{children}</div> }",
		"dashboard/page.tsx":     "export function Page() { return null }",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	routes, err := discoverRoutes(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeViews(root, root, routes); err != nil {
		t.Fatal(err)
	}
	view, err := os.ReadFile(filepath.Join(root, ".bifrost", "views", "page-0.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(view)
	for _, expected := range []string{
		"import Template1 from " + strconv.Quote(filepath.Join(root, "template.tsx")) + ";",
		"import { Template as Template3 } from " + strconv.Quote(filepath.Join(root, "dashboard", "template.tsx")) + ";",
		`<Layout0 key={"layout.tsx"} params={props.params}><Template1 key={pageKey} params={props.params}><Layout2 key={"dashboard/layout.tsx"} params={props.params}><Template3 key={pageKey} params={props.params}>`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("generated view does not contain %q:\n%s", expected, text)
		}
	}
}

func TestAppRouterViewsAcceptDefaultExports(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"page.tsx":           "export default function Page() { return null }",
		"layout.tsx":         "export default function Layout({ children }) { return children }",
		"error.tsx":          "export default function Error() { return null }",
		"not-found.tsx":      "export default function NotFound() { return null }",
		"dashboard/page.tsx": "export function Page() { return null }",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	routes, err := discoverRoutes(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeViews(root, root, routes); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".bifrost", "views"))
	if err != nil {
		t.Fatal(err)
	}
	var generated strings.Builder
	for _, entry := range entries {
		source, err := os.ReadFile(filepath.Join(root, ".bifrost", "views", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		generated.Write(source)
	}
	for _, expected := range []string{
		"import RoutePage from " + strconv.Quote(filepath.Join(root, "page.tsx")) + ";",
		"import Layout0 from " + strconv.Quote(filepath.Join(root, "layout.tsx")) + ";",
		"import ErrorPage0 from " + strconv.Quote(filepath.Join(root, "error.tsx")) + ";",
		"import NotFound from " + strconv.Quote(filepath.Join(root, "not-found.tsx")) + ";",
		"import { Page as RoutePage } from " + strconv.Quote(filepath.Join(root, "dashboard", "page.tsx")) + ";",
	} {
		if !strings.Contains(generated.String(), expected) {
			t.Fatalf("generated views do not contain %q:\n%s", expected, generated.String())
		}
	}
}

func TestAppRouterNotFoundRoutesUseTheURLPrefix(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"posts/not-found.tsx", "marketing~/not-found.tsx", "posts/api/not-found.tsx"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	routes, err := appendNotFoundRoutes(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	var patterns []string
	for _, route := range routes {
		patterns = append(patterns, route.Pattern)
	}
	if got := strings.Join(patterns, ","); got != "/posts/api/{path...},/posts/{path...},/{path...}" {
		t.Fatalf("patterns = %q", got)
	}
}

func TestGeneratedMainInjectsRequestProps(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, ".bifrost", "app")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	routes := []appRoute{
		{Pattern: "/posts/{post_id}", Params: []routeParam{{Name: "post-id", Value: "post_id"}}, View: "page.tsx", HasLoader: true, ImportPath: "example.com/app/posts", Alias: "route0", ErrorViews: []string{"error.tsx"}, NotFoundView: "not-found.tsx"},
		{Pattern: "/docs/{slug...}", Params: []routeParam{{Name: "slug", Value: "slug", Segments: true}}, View: "page.tsx"},
	}
	if err := writeAppRouterMain(root, generated, routes, nil, 0, 0); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(generated, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, expected := range []string{
		`requestPage(r, map[string]any{"post-id": r.PathValue("post_id")}, props, 1)`,
		`return requestPage(r, map[string]any{"slug": requestSegments(r.PathValue("slug"))}, nil, 0)`,
		`return requestPage(r, map[string]any{"post-id": r.PathValue("post_id")}, bifrost.PageData{Props: map[string]any{"__bifrostNotFound": true}, Status: status}, 1)`,
		`return requestPage(r, map[string]any{"post-id": r.PathValue("post_id")}, bifrost.PageData{Props: map[string]any{"__bifrostError": message, "__bifrostErrorLevel": 0}, Status: status}, 1)`,
		`values["params"] = params`,
		`values["searchParams"] = requestSearchParams(r)`,
		`values["pathname"] = r.URL.EscapedPath()`,
		`"strings"`,
		`reflect.ValueOf(props)`,
		`a page loader must return a map with string keys, or bifrost.PageData with one`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("generated main does not contain %q:\n%s", expected, text)
		}
	}
}

func TestGeneratedMainSetsRenderConcurrency(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, ".bifrost", "app")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	read := func() string {
		source, err := os.ReadFile(filepath.Join(generated, "main.go"))
		if err != nil {
			t.Fatal(err)
		}
		return string(source)
	}

	if err := writeAppRouterMain(root, generated, nil, nil, 4, 0); err != nil {
		t.Fatal(err)
	}
	if text := read(); !strings.Contains(text, "Assets: bifrostAssets, RenderConcurrency: 4, Routes:") {
		t.Fatalf("generated main does not set the render concurrency:\n%s", text)
	}

	if err := writeAppRouterMain(root, generated, nil, nil, 0, 0); err != nil {
		t.Fatal(err)
	}
	if text := read(); strings.Contains(text, "RenderConcurrency: ") {
		t.Fatalf("generated main sets a render concurrency with zero:\n%s", text)
	}
}

func TestGeneratedModuleUsesTheWorkingTreeForDevelopmentVersions(t *testing.T) {
	root := t.TempDir()
	userModule := filepath.Join(root, "app")
	if err := os.MkdirAll(userModule, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userModule, "go.mod"), []byte("module example.com/app\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	generated := filepath.Join(userModule, ".bifrost", "app")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	original := bifrost.Version
	t.Cleanup(func() { bifrost.Version = original })
	read := func() string {
		source, err := os.ReadFile(filepath.Join(generated, "go.mod"))
		if err != nil {
			t.Fatal(err)
		}
		return string(source)
	}

	development := []string{"devel", "devel+1a2b3c4d5e6f", "v1.3.11-0.20260924213727-f23f09ff9bad", "v0.0.0-20260924213727-f23f09ff9bad", "v1.3.11-0.20260924213727-f23f09ff9bad+dirty"}
	for _, version := range development {
		bifrost.Version = version
		if err := writeAppRouterModule(generated, userModule); err != nil {
			t.Fatal(err)
		}
		if source := read(); !strings.Contains(source, "replace github.com/3-lines-studio/bifrost => ") {
			t.Fatalf("version %q: generated module does not use the working tree:\n%s", version, source)
		}
	}

	bifrost.Version = "v1.3.10"
	if err := writeAppRouterModule(generated, userModule); err != nil {
		t.Fatal(err)
	}
	if source := read(); strings.Contains(source, "replace github.com/3-lines-studio/bifrost => ") {
		t.Fatalf("released version: generated module replaces the bifrost module:\n%s", source)
	}
}

func TestGeneratedMainSetsRenderQueue(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, ".bifrost", "app")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	read := func() string {
		source, err := os.ReadFile(filepath.Join(generated, "main.go"))
		if err != nil {
			t.Fatal(err)
		}
		return string(source)
	}

	if err := writeAppRouterMain(root, generated, nil, nil, 0, 512); err != nil {
		t.Fatal(err)
	}
	if text := read(); !strings.Contains(text, "Assets: bifrostAssets, RenderQueue: 512, Routes:") {
		t.Fatalf("generated main does not set the render queue:\n%s", text)
	}

	if err := writeAppRouterMain(root, generated, nil, nil, 0, 0); err != nil {
		t.Fatal(err)
	}
	if text := read(); strings.Contains(text, "RenderQueue: ") {
		t.Fatalf("generated main sets a render queue with zero:\n%s", text)
	}
}

func TestGeneratedMainDeclaresTheRouteCache(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, ".bifrost", "app")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	routes := []appRoute{
		{Pattern: "/cached", View: "page.tsx", Alias: "route0", ImportPath: "example.com/app/cached", HasCache: true},
		{Pattern: "/plain", View: "page.tsx", Alias: "route1", ImportPath: "example.com/app/plain"},
	}
	if err := writeAppRouterMain(root, generated, routes, nil, 0, 0); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(generated, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), ".WithNavigation().WithCache(route0.Cache()),") {
		t.Fatalf("generated main does not cache the declared route:\n%s", source)
	}
	if !strings.Contains(string(source), `bifrost.Server("/plain", "page.tsx", load1).WithNavigation(),`) {
		t.Fatalf("generated main caches a route that did not ask for it:\n%s", source)
	}
}

func TestGeneratedMainRenamesPathValuesForGo(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, ".bifrost", "app")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	routes := []appRoute{{Pattern: "/posts/{post_id}", Params: []routeParam{{Name: "post-id", Value: "post_id"}}, View: "page.tsx"}}
	if err := writeAppRouterMain(root, generated, routes, nil, 0, 0); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(generated, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), `r.SetPathValue("post-id", r.PathValue("post_id"))`) {
		t.Fatalf("generated main does not rename the path value:\n%s", source)
	}
}

func TestAppRouterLayoutsComposeOuterToInner(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"layout.tsx":                   "export function Layout({ children }) { return children }",
		"dashboard/layout.tsx":         "export function Layout({ children }) { return children }",
		"dashboard/settings/page.tsx":  "export function Head() { return null } export function Page() { return null }",
		"dashboard/settings/error.tsx": "export function Error() { return null }",
		"dashboard/not-found.tsx":      "export function NotFound() { return null }",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	routes, err := discoverRoutes(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeViews(root, root, routes); err != nil {
		t.Fatal(err)
	}
	view, err := os.ReadFile(filepath.Join(root, ".bifrost", "views", "page-0.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(view)
	if !strings.Contains(text, "export function renderPage") || !strings.Contains(text, `"use no memo";`) {
		t.Fatalf("generated tree factory is not hook-free:\n%s", text)
	}
	if !strings.Contains(text, "pageKey?: string") || !strings.Contains(text, "<Fragment key={pageKey}>") {
		t.Fatalf("generated page branch is not keyed:\n%s", text)
	}
	if !strings.Contains(text, `<Layout0 key={"layout.tsx"} params={props.params}><Layout1 key={"dashboard/layout.tsx"} params={props.params}>`) || !strings.Contains(text, "props.__bifrostError") || !strings.Contains(text, "props.__bifrostNotFound") || !strings.Contains(text, `export * from `) {
		t.Fatalf("generated view is incomplete:\n%s", text)
	}
}

func TestGeneratedModuleRequiresUserModule(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, ".bifrost", "app")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeAppRouterModule(generated, root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(generated, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "require example.com/app v0.0.0") || !strings.Contains(text, "replace example.com/app => "+strconv.Quote(filepath.ToSlash(root))) {
		t.Fatalf("generated module does not reference the user module:\n%s", text)
	}
}

func TestGoRouteRows(t *testing.T) {
	goDirs := []goDir{
		{Directory: ".", Pattern: "/{$}", Middleware: true},
		{Directory: "posts", Pattern: "/posts", Middleware: true},
		{Directory: "posts/api/slug_", Pattern: "/posts/api/{slug}", HTTPMethods: []string{"Get", "Post"}},
	}
	want := []routeRow{
		{kind: "middleware", pattern: "/*", source: "middleware.go"},
		{kind: "middleware", pattern: "/posts/*", source: "posts/middleware.go"},
		{kind: "api", pattern: "GET /posts/api/{slug}", source: "posts/api/slug_/route.go"},
		{kind: "api", pattern: "POST /posts/api/{slug}", source: "posts/api/slug_/route.go"},
	}
	if rows := goRouteRows(goDirs); !slices.Equal(rows, want) {
		t.Fatalf("goRouteRows() = %+v, want %+v", rows, want)
	}
	if rows := (*appRouter)(nil).routeRows(); rows != nil {
		t.Fatalf("nil app rows = %+v, want nil", rows)
	}
}

func TestGeneratedServerLifecycleAndEscapeHatch(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, ".bifrost", "app")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	routes := []appRoute{{Pattern: "/{$}", View: "page.tsx"}}
	goDirs := []goDir{{Directory: ".", ImportPath: "example.com/app", Alias: "route0", Serve: true}}
	if err := writeAppRouterMain(root, generated, routes, goDirs, 0, 0); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(generated, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, expected := range []string{"signal.NotifyContext", "ReadHeaderTimeout", "BIFROST_ADDR", `flag.StringVar(&addr, "addr"`, "route0.Serve(ctx, redirectTrailingSlash(app.ResolveMarkdown(mux)))", "server.Shutdown"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("generated main does not contain %q", expected)
		}
	}
	if strings.Contains(text, "WriteTimeout") {
		t.Fatal("generated server sets WriteTimeout")
	}
	goDirs[0].Serve = false
	if err := writeAppRouterMain(root, generated, routes, goDirs, 0, 0); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(generated, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "return serve(ctx, redirectTrailingSlash(app.ResolveMarkdown(mux)))") {
		t.Fatal("generated main does not apply markdown resolution")
	}
}

func TestDirectoryContains(t *testing.T) {
	if !directoryContains(".", "dashboard/settings") || !directoryContains("dashboard", "dashboard/settings") || directoryContains("dash", "dashboard") {
		t.Fatal("unexpected middleware ancestry")
	}
}

func TestAppRouterRootsAcceptAPIOnlyApps(t *testing.T) {
	projectRoot := t.TempDir()
	appRoot := filepath.Join(projectRoot, "app", "api", "hello_")
	if err := os.MkdirAll(appRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appRoot, "route.go"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	project, routes, ok, err := appRouterRoots(".", projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || project != projectRoot || routes != filepath.Join(projectRoot, "app") {
		t.Fatalf("routes without pages = %q, %q, %t", project, routes, ok)
	}
}

func TestAppRouterRootsAcceptMarkerDirectoriesWithoutPages(t *testing.T) {
	projectRoot := t.TempDir()
	marker := filepath.Join(projectRoot, "posts", "slug_")
	if err := os.MkdirAll(marker, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(marker, "route.go"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	project, routes, ok, err := appRouterRoots(".", projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || project != projectRoot || routes != projectRoot {
		t.Fatalf("roots with a marker directory = %q, %q, %t", project, routes, ok)
	}
}

func TestAppRouterRootsIgnorePlainGoFiles(t *testing.T) {
	projectRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, "internal", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "main.go"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "internal", "api", "route.go"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := appRouterRoots(".", projectRoot); err != nil || ok {
		t.Fatalf("plain Go files were taken as an App Router app: ok=%t err=%v", ok, err)
	}
}

func TestPrepareAppRouterAppAcceptsRoutesWithoutPages(t *testing.T) {
	projectRoot := t.TempDir()
	appRoot := filepath.Join(projectRoot, "app")
	routeRoot := filepath.Join(appRoot, "api", "hello_")
	if err := os.MkdirAll(routeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "go.mod"), []byte("module api.test/app\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(routeRoot, "route.go"), []byte("package api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app, err := prepareAppRouter(context.Background(), projectRoot, appRoot, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if app.Executable != filepath.Join(projectRoot, ".bifrost", "bifrost-app") {
		t.Fatalf("executable = %q", app.Executable)
	}
	if _, err := os.Stat(filepath.Join(app.WorkDir, "main.go")); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareAppRouterAppRejectsEmptyTrees(t *testing.T) {
	projectRoot := t.TempDir()
	appRoot := filepath.Join(projectRoot, "app")
	if err := os.MkdirAll(appRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareAppRouter(context.Background(), projectRoot, appRoot, 0, 0); err == nil {
		t.Fatal("an empty route root was accepted")
	}
}
