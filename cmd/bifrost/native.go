package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
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

const nativePackageJSON = `{
  "private": true,
  "main": "expo-router/entry",
  "dependencies": {
    "expo": "~57.0.28",
    "expo-constants": "~57.0.21",
    "expo-linking": "~57.0.12",
    "expo-router": "~57.0.26",
    "expo-status-bar": "~57.0.1",
    "react": "19.2.3",
    "react-native": "0.86.3",
    "react-native-safe-area-context": "5.10.1",
    "react-native-screens": "4.29.0"
  },
  "devDependencies": {
    "@types/react": "~19.2.2",
    "typescript": "~6.0.3"
  }
}
`

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

config.resolver.resolveRequest = (context, moduleName, platform) => {
  if (moduleName === "virtual:bifrost/navigation") {
    return { type: "sourceFile", filePath: navigation };
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

export async function loadPage(pathname: string): Promise<PageData> {
  const response = await fetch(ORIGIN + pathname, { headers: { Accept: DATA_MEDIA_TYPE } });
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
  return body as PageData;
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

const nativeScreen = `import { useGlobalSearchParams, usePathname } from "expo-router";
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
  const search = useGlobalSearchParams() as Record<string, unknown>;
  const [state, setState] = useState<State>({ status: "loading" });
  const [nonce, setNonce] = useState(0);
  const href = pathname + query(search, params);

  useEffect(() => onRefresh(() => { setNonce((value) => value + 1); }), []);

  useEffect(() => {
    let live = true;
    setState({ status: "loading" });
    loadPage(href).then(
      (data) => { if (live) setState({ status: "ready", data }); },
      (failure) => { if (live) setState({ status: "failed", message: String(failure) }); },
    );
    return () => { live = false; };
  }, [href, nonce]);

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

var nativeProjectFiles = []nativeFile{
	{"package.json", nativePackageJSON},
	{"tsconfig.json", nativeTSConfig},
	{"metro.config.js", nativeMetroConfig},
	{"app/_layout.tsx", nativeLayout},
	{"bifrost/data.ts", nativeData},
	{"bifrost/refresh.ts", nativeRefresh},
	{"bifrost/navigation.tsx", nativeNavigation},
	{"bifrost/screen.tsx", nativeScreen},
}

func runNative(args []string) error {
	flags := flag.NewFlagSet("native", flag.ContinueOnError)
	dir := flags.String("C", ".", "working directory")
	origin := flags.String("origin", "", "base URL of the Go server; by default the host alias of each platform")
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
	if err := writeNativeProject(native, prepared, *origin); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(os.Stdout, "Bifrost native project: %s\n", filepath.ToSlash(native))
	_, _ = fmt.Fprintf(os.Stdout, "  cd %s && bun install && bunx expo start\n", filepath.ToSlash(native))
	return nil
}

func writeNativeProject(native string, app appRouter, origin string) error {
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
	name, slug := nativeNames(app.ProjectRoot)
	manifest, err := json.MarshalIndent(map[string]any{"expo": map[string]any{
		"name":               name,
		"slug":               slug,
		"scheme":             slug,
		"version":            "1.0.0",
		"orientation":        "portrait",
		"userInterfaceStyle": "automatic",
		"newArchEnabled":     true,
		"plugins":            []string{"expo-router"},
		"ios":                map[string]any{"bundleIdentifier": "com.bifrost." + strings.ReplaceAll(slug, "-", "")},
		"android":            map[string]any{"package": "com.bifrost." + strings.ReplaceAll(slug, "-", "")},
	}}, "", "  ")
	if err != nil {
		return err
	}
	if err := writeNativeFile(native, "app.json", string(manifest)+"\n", 0o644); err != nil {
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
