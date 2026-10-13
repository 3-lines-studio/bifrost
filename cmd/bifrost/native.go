package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// The Expo project a native target needs. Every file here is regenerated, so
// the project tree stays a build artifact of the App Router tree and never a
// second source of truth.
type nativeFile struct {
	Name    string
	Content string
}

const nativeTSConfig = `{
  "extends": "expo/tsconfig.base",
  "compilerOptions": {
    "strict": true
  }
}
`

const nativeMetroConfig = `const path = require("node:path");
const { getDefaultConfig } = require("expo/metro-config");

const projectRoot = __dirname;
const workspaceRoot = path.resolve(projectRoot, "../..");

const config = getDefaultConfig(projectRoot);
config.watchFolders = [workspaceRoot];
config.resolver.nodeModulesPaths = [
  path.join(projectRoot, "node_modules"),
  path.join(workspaceRoot, "node_modules"),
];

const navigation = path.join(projectRoot, "bifrost", "navigation.tsx");
const runtime = path.join(projectRoot, "bifrost", "jsx-runtime.ts");

config.resolver.resolveRequest = (context, moduleName, platform) => {
  if (moduleName === "virtual:bifrost/navigation") {
    return { type: "sourceFile", filePath: navigation };
  }
  // Every element the compiler builds goes through the runtime, which is where a
  // tag written for the web becomes the component that paints it natively. The
  // wrapper needs the real one, so its own import is left alone.
  const runtimeImport =
    moduleName === "react/jsx-runtime" || moduleName === "react/jsx-dev-runtime";
  if (runtimeImport && !context.originModulePath.startsWith(path.join(projectRoot, "bifrost"))) {
    return { type: "sourceFile", filePath: runtime };
  }
  return context.resolveRequest(context, moduleName, platform);
};

module.exports = config;
`

const nativeLayout = `import { Stack } from "expo-router";
import { StatusBar } from "expo-status-bar";
import { StyleSheet, View } from "react-native";
import { SafeAreaProvider, useSafeAreaInsets } from "react-native-safe-area-context";

function Shell() {
  const insets = useSafeAreaInsets();
  return (
    <View style={[styles.shell, { paddingTop: insets.top, paddingBottom: insets.bottom }]}>
      <Stack screenOptions={{ headerShown: false }} />
    </View>
  );
}

export default function Layout() {
  return (
    <SafeAreaProvider>
      <StatusBar style="auto" />
      <Shell />
    </SafeAreaProvider>
  );
}

const styles = StyleSheet.create({
  shell: { flex: 1 },
});
`

const nativeData = `import { ORIGIN } from "./origin";

export const DATA_MEDIA_TYPE = "application/vnd.bifrost.data+json";

export type PageData = {
  pattern: string;
  props: Record<string, unknown>;
  document: { lang: string; class?: string; dir?: string };
  status: number;
};

export type PageResult =
  | { kind: "page"; data: PageData }
  | { kind: "redirect"; href: string };

export async function loadPage(pathname: string): Promise<PageResult> {
  // A server answers a redirect before the route runs: a session middleware
  // pushing an anonymous client to the login page. The runtime follows it and
  // only reports where it landed, so a moved url is the redirect, and rendering
  // the destination under the path that was asked for would be a silent lie.
  const response = await fetch(ORIGIN + pathname, { headers: { Accept: DATA_MEDIA_TYPE } });
  const target = redirectTarget(response, pathname);
  if (target !== undefined) {
    return { kind: "redirect", href: target };
  }
  const type = response.headers.get("content-type") || "";
  if (type.indexOf(DATA_MEDIA_TYPE) < 0) {
    throw new Error("bifrost: " + pathname + " answered " + response.status + " with " + (type || "no content type"));
  }
  const body = await response.json();
  if (body && typeof body.error === "string") {
    throw new Error("bifrost: " + pathname + " " + body.status + " " + body.error);
  }
  if (!body || typeof body.props !== "object" || body.props === null) {
    throw new Error("bifrost: " + pathname + " answered without props");
  }
  return { kind: "page", data: body as PageData };
}

function redirectTarget(response: Response, pathname: string): string | undefined {
  // The url is compared by path, so a runtime that normalizes it cannot be
  // mistaken for a redirect and send the router in a circle.
  if (response.url) {
    const landed = appHref(response.url);
    if (landed !== pathname) {
      return landed;
    }
  }
  if (response.status >= 300 && response.status < 400) {
    const location = response.headers.get("location");
    if (location) {
      return appHref(location);
    }
  }
  return undefined;
}

function appHref(location: string): string {
  if (location.charAt(0) === "/") {
    return location;
  }
  if (location.indexOf(ORIGIN) === 0) {
    return location.slice(ORIGIN.length) || "/";
  }
  throw new Error("bifrost: the server answered a url outside the app: " + location);
}
`

const nativeRefresh = `const listeners = new Set<() => void>();

export function refresh(): Promise<void> {
  for (const listener of listeners) {
    listener();
  }
  return Promise.resolve();
}

export function onRefresh(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
`

const nativeNavigation = `import { createContext, useContext, type ReactNode } from "react";
import { Pressable, Text } from "react-native";
import { router } from "expo-router";
import { refresh } from "./refresh";

export { refresh };

type RouteValue = {
  pathname: string;
  params: Record<string, string | string[]>;
  searchParams: URLSearchParams;
};

const RouteContext = createContext<RouteValue>({
  pathname: "/",
  params: {},
  searchParams: new URLSearchParams(),
});

function toSearchParams(value: unknown): URLSearchParams {
  const out = new URLSearchParams();
  if (value && typeof value === "object") {
    for (const [key, item] of Object.entries(value as Record<string, unknown>)) {
      if (Array.isArray(item)) {
        item.forEach((one) => { out.append(key, String(one)); });
        continue;
      }
      if (item !== undefined && item !== null) {
        out.append(key, String(item));
      }
    }
  }
  return out;
}

export function RouteProvider({ pathname, params, searchParams, children }: { pathname?: unknown; params?: unknown; searchParams?: unknown; children?: ReactNode }) {
  const value: RouteValue = {
    pathname: typeof pathname === "string" ? pathname : "/",
    params: (params ?? {}) as Record<string, string | string[]>,
    searchParams: toSearchParams(searchParams),
  };
  return <RouteContext.Provider value={value}>{children}</RouteContext.Provider>;
}

export function usePathname(): string {
  return useContext(RouteContext).pathname;
}

export function useParams(): Record<string, string | string[]> {
  return useContext(RouteContext).params;
}

export function useSearchParams(): URLSearchParams {
  return useContext(RouteContext).searchParams;
}

export async function navigate(href: string): Promise<void> {
  router.push(href as never);
}

export async function replace(href: string): Promise<void> {
  router.replace(href as never);
}

export function Link({ href, children, ...rest }: { href: string; children?: ReactNode } & Record<string, unknown>) {
  return (
    <Pressable accessibilityRole="link" onPress={() => { void navigate(href); }} {...rest}>
      <Text>{children}</Text>
    </Pressable>
  );
}

export function useRouter() {
  return {
    push: navigate,
    replace,
    refresh,
    back: () => { router.back(); },
    forward: () => { router.forward(); },
  };
}
`

const nativeScreen = `import { router, useIsFocused, useLocalSearchParams, usePathname } from "expo-router";
import { useEffect, useState } from "react";
import { ActivityIndicator, StyleSheet, Text, View } from "react-native";
import { loadPage, type PageData } from "./data";
import { onRefresh } from "./refresh";

type State =
  | { status: "loading" }
  | { status: "ready"; data: PageData }
  | { status: "failed"; message: string };

export type ScreenProps = {
  render: (props: Record<string, unknown>, pageKey?: string) => unknown;
  params: string[];
};

export function Screen({ render, params }: ScreenProps) {
  const pathname = usePathname();
  // A stack keeps the screens behind the current one mounted, so the global
  // params belong to whichever route is on top. Reading them here would give
  // this screen another route's url, and it would fetch and render the page
  // that route points at, under a pathname that is no longer this one.
  const search = useLocalSearchParams() as Record<string, unknown>;
  const focused = useIsFocused();
  const [state, setState] = useState<State>({ status: "loading" });
  const [nonce, setNonce] = useState(0);
  const href = pathname + query(search, params);

  useEffect(() => onRefresh(() => { setNonce((value) => value + 1); }), []);

  useEffect(() => {
    if (!focused) {
      return;
    }
    let live = true;
    setState({ status: "loading" });
    loadPage(href).then(
      (result) => {
        if (!live) {
          return;
        }
        if (result.kind === "redirect") {
          router.replace(result.href as never);
          return;
        }
        setState({ status: "ready", data: result.data });
      },
      (failure) => { if (live) setState({ status: "failed", message: String(failure) }); },
    );
    return () => { live = false; };
  }, [href, nonce, focused]);

  if (state.status === "loading") {
    return <View style={styles.center}><ActivityIndicator /></View>;
  }
  if (state.status === "failed") {
    return <View style={styles.center}><Text style={styles.error}>{state.message}</Text></View>;
  }
  const props = state.data.props;
  return <>{render(props, typeof props.pathname === "string" ? props.pathname : pathname)}</>;
}

function query(search: Record<string, unknown>, params: string[]): string {
  const values = new URLSearchParams();
  for (const [key, value] of Object.entries(search)) {
    if (params.indexOf(key) >= 0 || value === undefined || value === null) {
      continue;
    }
    if (Array.isArray(value)) {
      value.forEach((one) => { values.append(key, String(one)); });
      continue;
    }
    values.append(key, String(value));
  }
  const encoded = values.toString();
  return encoded === "" ? "" : "?" + encoded;
}

const styles = StyleSheet.create({
  center: { flex: 1, alignItems: "center", justifyContent: "center", padding: 24 },
  error: { color: "crimson", fontSize: 12, textAlign: "center" },
});
`

// A page is written once, for the browser, and the tags in it are what the
// screen paints. This table is the vocabulary a native target supports, so the
// runtime and the tests read the same list.
const nativeHost = `import { Children, isValidElement, type ReactNode } from "react";
import { Image, Linking, Pressable, Text, TextInput, View } from "react-native";
import { navigate } from "virtual:bifrost/navigation";

// Blocks. They hold other blocks, which a View can.
const BOXES = ["address", "article", "aside", "caption", "dd", "details", "dialog", "div", "dl", "dt", "fieldset", "figcaption", "figure", "footer", "form", "header", "legend", "li", "main", "nav", "ol", "output", "section", "summary", "table", "tbody", "td", "tfoot", "th", "thead", "tr", "ul"];

// Everything whose content is text.
const TEXTS = ["abbr", "b", "br", "cite", "code", "em", "h1", "h2", "h3", "h4", "h5", "h6", "i", "label", "mark", "p", "pre", "q", "s", "small", "span", "strong", "time", "u"];

// Sheet links, metadata and scripts have nothing to paint.
const HIDDEN = ["base", "head", "link", "meta", "noscript", "script", "style", "title"];

// Only a Text may hold a string, so a block that holds text has to become one.
// A run of text and inline elements is wrapped together; everything else keeps
// its own component, which is what a block child already is.
function Box({ children, ...rest }: { children?: ReactNode } & Record<string, unknown>) {
  const runs: ReactNode[] = [];
  let text: ReactNode[] = [];
  const close = () => {
    if (text.length > 0) {
      runs.push(<Text key={"text-" + runs.length}>{text}</Text>);
      text = [];
    }
  };
  for (const child of Children.toArray(children)) {
    if (typeof child === "string" || typeof child === "number" || (isValidElement(child) && child.type === Text)) {
      text.push(child);
      continue;
    }
    close();
    runs.push(child);
  }
  close();
  return <View {...rest}>{runs}</View>;
}

function Anchor({ href, children, ...rest }: { href?: string; children?: unknown } & Record<string, unknown>) {
  return (
    <Pressable
      accessibilityRole="link"
      onPress={() => {
        if (typeof href !== "string") {
          return;
        }
        if (href.charAt(0) === "/") {
          void navigate(href);
          return;
        }
        void Linking.openURL(href);
      }}
      {...rest}
    >
      <Text>{children}</Text>
    </Pressable>
  );
}

function Button({ children, ...rest }: { children?: unknown } & Record<string, unknown>) {
  return <Pressable accessibilityRole="button" {...rest}><Text>{children}</Text></Pressable>;
}

function Input({ value, defaultValue, ...rest }: Record<string, unknown>) {
  return <TextInput defaultValue={defaultValue === undefined ? (value as string) : (defaultValue as string)} {...rest} />;
}

function ImageTag({ src, alt, ...rest }: { src?: string; alt?: string } & Record<string, unknown>) {
  return <Image source={{ uri: typeof src === "string" ? src : undefined }} accessibilityLabel={alt} {...rest} />;
}

function Rule() {
  return <View style={{ height: 1, backgroundColor: "#e9dfd2" }} />;
}

function Nothing() {
  return null;
}

const NAMED: Record<string, unknown> = {
  a: Anchor,
  button: Button,
  hr: Rule,
  img: ImageTag,
  input: Input,
  textarea: TextInput,
};
for (const name of BOXES) {
  NAMED[name] = Box;
}
for (const name of TEXTS) {
  NAMED[name] = Text;
}
for (const name of HIDDEN) {
  NAMED[name] = Nothing;
}

// React Native builds its own views out of strings too, so a name this table
// does not know has to stay exactly as it is: replacing it would tear the
// native views apart.
export function hostComponent(name: string): unknown {
  const component = NAMED[name];
  if (component !== undefined) {
    return component;
  }
  // A name that is all lowercase is a tag this table is missing, and it is
  // about to fail deep inside the renderer. Say which one.
  if (__DEV__ && name === name.toLowerCase()) {
    console.warn("bifrost: no native counterpart for <" + name + ">");
  }
  return undefined;
}
`

const nativeJSXRuntime = `import * as runtime from "react/jsx-runtime";

// This module is evaluated before the app's own runtime is ready, because the
// compiler needs it to build the first element. It must therefore pull nothing
// in by itself: importing react-native here would land it ahead of the URL and
// TextEncoder patches expo installs, and the app would die before it renders.
type Host = { hostComponent(name: string): unknown };

let host: Host | null = null;

function component(name: string): unknown {
  if (host === null) {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    host = require("./host") as Host;
  }
  const mapped = host.hostComponent(name);
  return mapped === undefined ? name : mapped;
}

// A tag a page wrote for the web becomes the component that paints it natively;
// a name this table does not know is left alone, because react-native builds
// its own views out of strings too.
const wrap = (create: unknown) =>
  (type: unknown, ...rest: unknown[]) =>
    (create as (type: unknown, ...rest: unknown[]) => unknown)(
      typeof type === "string" ? component(type) : type,
      ...rest,
    );

export const jsx = wrap(runtime.jsx);
export const jsxs = wrap(runtime.jsxs);
export const jsxDEV = wrap(runtime.jsxDEV);
export const Fragment = runtime.Fragment;
`

var nativeProjectFiles = []nativeFile{
	{"tsconfig.json", nativeTSConfig},
	{"metro.config.js", nativeMetroConfig},
	{"app/_layout.tsx", nativeLayout},
	{"bifrost/data.ts", nativeData},
	{"bifrost/host.tsx", nativeHost},
	{"bifrost/jsx-runtime.ts", nativeJSXRuntime},
	{"bifrost/refresh.ts", nativeRefresh},
	{"bifrost/navigation.tsx", nativeNavigation},
	{"bifrost/screen.tsx", nativeScreen},
}

func nativePackageManifest() map[string]any {
	return map[string]any{
		"private": true,
		"main":    "expo-router/entry",
		"dependencies": map[string]any{
			"expo":                           "~57.0.28",
			"expo-constants":                 "~57.0.21",
			"expo-linking":                   "~57.0.12",
			"expo-router":                    "~57.0.26",
			"expo-status-bar":                "~57.0.1",
			"react":                          "19.2.3",
			"react-native":                   "0.86.3",
			"react-native-safe-area-context": "5.10.1",
			"react-native-screens":           "4.29.0",
		},
		"devDependencies": map[string]any{
			"@types/react": "~19.2.2",
			"typescript":   "~6.0.3",
		},
	}
}

func nativeAppManifest(projectRoot string) map[string]any {
	name, slug := nativeNames(projectRoot)
	identifier := "com.bifrost." + strings.ReplaceAll(slug, "-", "")
	return map[string]any{"expo": map[string]any{
		"name":               name,
		"slug":               slug,
		"scheme":             slug,
		"version":            "1.0.0",
		"orientation":        "portrait",
		"userInterfaceStyle": "automatic",
		"newArchEnabled":     true,
		"plugins":            []string{"expo-router"},
		"ios":                map[string]any{"bundleIdentifier": identifier},
		"android":            map[string]any{"package": identifier},
	}}
}

func runNative(args []string) error {
	flags := flag.NewFlagSet("native", flag.ContinueOnError)
	dir := flags.String("C", ".", "working directory")
	origin := flags.String("origin", "", "base URL of the Go server; by default the host alias of each platform")
	from := flags.String("from", "native", "directory of app files copied over the generated project")
	if err := flags.Parse(args); err != nil {
		return err
	}
	packagePath := "."
	if flags.NArg() > 0 {
		packagePath = flags.Arg(0)
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("native accepts one package path")
	}
	ctx := context.Background()
	projectRoot, routeRoot, isAppRouter, err := appRouterRoots(*dir, packagePath)
	if err != nil {
		return err
	}
	if !isAppRouter {
		return errors.New("bifrost: native requires an App Router app")
	}
	prepared, err := prepareAppRouter(ctx, projectRoot, routeRoot, 0, 0)
	if err != nil {
		return err
	}
	native := filepath.Join(projectRoot, ".bifrost", "native")
	overlay := filepath.Join(projectRoot, *from)
	if err := writeNativeProject(native, prepared, *origin, overlay); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(os.Stdout, "Bifrost native project: %s\n", filepath.ToSlash(native))
	if overlayExists(overlay) {
		_, _ = fmt.Fprintf(os.Stdout, "  app files from: %s\n", filepath.ToSlash(overlay))
	}
	_, _ = fmt.Fprintf(os.Stdout, "  cd %s && bun install && bunx expo start\n", filepath.ToSlash(native))
	return nil
}

func writeNativeProject(native string, app appRouter, origin, overlay string) error {
	screens := make(map[string]appRoute, len(app.Routes))
	for _, route := range app.Routes {
		view, err := expoRouterPath(route.Pattern)
		if err != nil {
			return err
		}
		if previous, exists := screens[view]; exists {
			return fmt.Errorf("bifrost: routes %s and %s both map to native path %s", previous.Pattern, route.Pattern, view)
		}
		screens[view] = route
	}
	for _, name := range []string{"app", "bifrost"} {
		if err := os.RemoveAll(filepath.Join(native, name)); err != nil {
			return err
		}
	}
	for _, file := range nativeProjectFiles {
		if err := writeNativeFile(native, file.Name, file.Content, 0o644); err != nil {
			return err
		}
	}
	manifest := nativePackageManifest()
	if err := mergeNativeOverlayManifest(overlay, manifest); err != nil {
		return err
	}
	if err := writeNativeJSON(native, "package.json", manifest); err != nil {
		return err
	}
	if err := writeNativeJSON(native, "app.json", nativeAppManifest(app.ProjectRoot)); err != nil {
		return err
	}
	if err := writeNativeFile(native, "bifrost/origin.ts", nativeOrigin(origin), 0o644); err != nil {
		return err
	}
	views := filepath.Join(app.ProjectRoot, ".bifrost", "views")
	for view, route := range screens {
		source, err := screenSource(native, views, view, route, app.RouteParams[route.Pattern])
		if err != nil {
			return err
		}
		if err := writeNativeFile(native, filepath.Join("app", filepath.FromSlash(view)+".tsx"), source, 0o644); err != nil {
			return err
		}
	}
	return copyNativeOverlay(native, overlay, screens)
}

// App files are copied last, so an app owns its screens, its layout and the
// client it talks to the server with, while the route table keeps owning which
// routes exist.
func copyNativeOverlay(native, overlay string, screens map[string]appRoute) error {
	names, err := nativeOverlayFiles(overlay)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := checkNativeOverlayRoute(name, screens); err != nil {
			return err
		}
		if name == "package.json" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(overlay, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		if err := writeNativeFile(native, name, string(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// The web tree and the native tree come from one route table, so an app may
// restyle a screen but never add one the server does not serve.
func checkNativeOverlayRoute(name string, screens map[string]appRoute) error {
	rest, ok := strings.CutPrefix(name, "app/")
	if !ok || rest == "_layout.tsx" {
		return nil
	}
	if _, exists := screens[strings.TrimSuffix(rest, ".tsx")]; exists {
		return nil
	}
	return fmt.Errorf("bifrost: %s is not a route of the App Router tree", name)
}

func nativeOverlayFiles(overlay string) ([]string, error) {
	if !overlayExists(overlay) {
		return nil, nil
	}
	var names []string
	err := filepath.WalkDir(overlay, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("bifrost: native file symlink %q is not allowed", path)
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(overlay, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func overlayExists(overlay string) bool {
	info, err := os.Stat(overlay)
	return err == nil && info.IsDir()
}

// An app adds the modules its own screens need to the manifest the generator
// owns; every other key stays generated.
func mergeNativeOverlayManifest(overlay string, manifest map[string]any) error {
	content, err := os.ReadFile(filepath.Join(overlay, "package.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var declared map[string]any
	if err := json.Unmarshal(content, &declared); err != nil {
		return fmt.Errorf("bifrost: native package.json: %w", err)
	}
	for _, key := range []string{"dependencies", "devDependencies"} {
		added, ok := declared[key].(map[string]any)
		if !ok {
			continue
		}
		base, _ := manifest[key].(map[string]any)
		for name, version := range added {
			base[name] = version
		}
		manifest[key] = base
	}
	return nil
}

func screenSource(native, views, view string, route appRoute, params []string) (string, error) {
	directory := filepath.Dir(filepath.Join(native, "app", filepath.FromSlash(view)))
	screen, err := nativeImport(directory, filepath.Join(native, "bifrost", "screen"))
	if err != nil {
		return "", err
	}
	page, err := nativeImport(directory, filepath.Join(views, filepath.Base(filepath.FromSlash(route.View))))
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(params))
	for _, name := range params {
		names = append(names, strconv.Quote(name))
	}
	return "import { Screen } from " + strconv.Quote(screen) + ";\n" +
		"import { renderPage } from " + strconv.Quote(page) + ";\n\n" +
		"export default function Route() {\n" +
		"  return <Screen render={renderPage} params={[" + strings.Join(names, ", ") + "]} />;\n" +
		"}\n", nil
}

func nativeImport(from, target string) (string, error) {
	relative, err := filepath.Rel(from, target)
	if err != nil {
		return "", err
	}
	relative = filepath.ToSlash(relative)
	if !strings.HasPrefix(relative, ".") {
		relative = "./" + relative
	}
	return relative, nil
}

func nativeOrigin(origin string) string {
	if origin != "" {
		return "export const ORIGIN = " + strconv.Quote(strings.TrimSuffix(origin, "/")) + ";\n"
	}
	return `import { Platform } from "react-native";

export const ORIGIN =
  Platform.OS === "android" ? "http://10.0.2.2:8080" : "http://localhost:8080";
`
}

func nativeNames(projectRoot string) (string, string) {
	base := strings.ToLower(filepath.Base(projectRoot))
	var slug strings.Builder
	for _, char := range base {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			slug.WriteRune(char)
			continue
		}
		if slug.Len() > 0 && !strings.HasSuffix(slug.String(), "-") {
			slug.WriteByte('-')
		}
	}
	cleaned := strings.Trim(slug.String(), "-")
	if cleaned == "" {
		cleaned = "bifrost-app"
	}
	return filepath.Base(projectRoot), cleaned
}

// expoRouterPath turns a ServeMux pattern into the Expo Router file that
// catches it, so both routers agree because they come from the same table.
func expoRouterPath(pattern string) (string, error) {
	segments := make([]string, 0, 4)
	for _, segment := range strings.Split(pattern, "/") {
		if segment == "" || segment == "{$}" {
			continue
		}
		segments = append(segments, expoRouterSegment(segment))
	}
	if len(segments) == 0 {
		return "index", nil
	}
	return strings.Join(segments, "/"), nil
}

func expoRouterSegment(segment string) string {
	if !strings.HasPrefix(segment, "{") || !strings.HasSuffix(segment, "}") {
		return segment
	}
	name := strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}")
	if strings.HasSuffix(name, "...") {
		return "[..." + strings.TrimSuffix(name, "...") + "]"
	}
	return "[" + name + "]"
}

func writeNativeFile(native, name, content string, mode os.FileMode) error {
	filePath := filepath.Join(native, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filePath, []byte(content), mode)
}

func writeNativeJSON(native, name string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeNativeFile(native, name, string(content)+"\n", 0o644)
}
