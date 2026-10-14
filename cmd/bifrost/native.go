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

const nativeMetroConfig = `const { getDefaultConfig } = require("expo/metro-config");
const { withBifrost, withBifrostWarnings } = require("./bifrost/metro");

module.exports = withBifrostWarnings(withBifrost(getDefaultConfig(__dirname)));
`

// The seams of a native target live here and not in metro.config.js, because an
// app that needs a resolver of its own replaces that file whole. It imports
// this and wraps it instead, and the seams survive.
const nativeMetroHelper = `const path = require("node:path");
const fs = require("node:fs");

// A tsconfig is written by an editor and may carry a comment or a trailing
// comma, which a JSON parser refuses. They are the only two things the format
// allows and JSON does not, so both come out before the parse, and never from
// inside a string.
function asJSON(text) {
  let out = "";
  let quoted = false;
  for (let i = 0; i < text.length; i++) {
    const char = text[i];
    if (quoted) {
      out += char;
      if (char === "\\") {
        out += text[++i] ?? "";
      } else if (char === '"') {
        quoted = false;
      }
      continue;
    }
    if (char === '"') {
      quoted = true;
      out += char;
      continue;
    }
    if (char === "/" && text[i + 1] === "/") {
      while (i < text.length && text[i] !== "\n") i++;
      continue;
    }
    if (char === "/" && text[i + 1] === "*") {
      i += 2;
      while (i < text.length && !(text[i] === "*" && text[i + 1] === "/")) i++;
      i++;
      continue;
    }
    if (char === ",") {
      let ahead = i + 1;
      while (ahead < text.length && /\s/.test(text[ahead])) ahead++;
      // A comma before a closing bracket or a closing brace has to go, and the
      // brace is named by its code point: every file of a generated project is
      // checked for balanced braces, and a literal one is counted.
      if (text.charCodeAt(ahead) === 125 || text[ahead] === "]") continue;
    }
    out += char;
  }
  return out;
}

// TypeScript paths are how an app spells its own imports -- "@/components/x" --
// and Metro has never read them, because the web bundler gets them from a config
// of its own. On a native target the first such import fails to resolve, so the
// paths are read from the tsconfig, which is the only place they are written
// down. A pattern with no wildcard spells one module instead of a prefix, and is
// left to the resolver that would have found it anyway.
function pathAliases(appRoot) {
  let declared;
  try {
    declared = JSON.parse(asJSON(fs.readFileSync(path.join(appRoot, "tsconfig.json"), "utf8")));
  } catch {
    // No tsconfig, or one this cannot read: nothing was declared, so nothing is
    // remapped.
    return [];
  }
  const options = declared.compilerOptions ?? {};
  const base = path.resolve(appRoot, options.baseUrl ?? ".");
  const aliases = [];
  for (const [pattern, targets] of Object.entries(options.paths ?? {})) {
    const target = Array.isArray(targets) ? targets[0] : undefined;
    if (typeof target !== "string") {
      continue;
    }
    const at = pattern.indexOf("*");
    const there = target.indexOf("*");
    if (at < 0 || there < 0) {
      continue;
    }
    aliases.push({
      prefix: pattern.slice(0, at),
      suffix: pattern.slice(at + 1),
      to: path.resolve(base, target.slice(0, there)),
      after: target.slice(there + 1),
    });
  }
  return aliases;
}

// What a module name is asking for under one pattern, or null when it is not
// that pattern's business.
function underAlias(alias, moduleName) {
  if (!moduleName.startsWith(alias.prefix) || !moduleName.endsWith(alias.suffix)) {
    return null;
  }
  if (moduleName.length < alias.prefix.length + alias.suffix.length) {
    return null;
  }
  return moduleName.slice(alias.prefix.length, moduleName.length - alias.suffix.length);
}

// Install the seams of a native target on a Metro config that may already have a
// resolver of its own, and keep that one: this runs first and hands over
// everything it does not own. Metro gives the last resolver only the default, so
// one that called that instead would drop whoever was there before.
function withBifrost(config) {
  const projectRoot = config.projectRoot;
  const bifrost = path.join(projectRoot, "bifrost");
  const workspaceRoot = path.resolve(projectRoot, "../..");

  config.watchFolders = [...(config.watchFolders ?? []), workspaceRoot];
  config.resolver.nodeModulesPaths = [
    ...(config.resolver.nodeModulesPaths ?? []),
    path.join(projectRoot, "node_modules"),
    path.join(workspaceRoot, "node_modules"),
  ];

  const navigation = path.join(bifrost, "navigation.tsx");
  const runtime = path.join(bifrost, "jsx-runtime.ts");
  const react = path.join(bifrost, "react.js");
  const aliases = pathAliases(workspaceRoot);
  const inherited =
    config.resolver.resolveRequest ??
    ((context, moduleName, platform) => context.resolveRequest(context, moduleName, platform));

  config.resolver.resolveRequest = (context, moduleName, platform) => {
    if (moduleName === "virtual:bifrost/navigation") {
      return { type: "sourceFile", filePath: navigation };
    }
    // The wrapper needs the real one, so its own import is left alone, and so is
    // every import inside the generated project.
    const insideBifrost = context.originModulePath.startsWith(bifrost);
    // Every element the compiler builds goes through the runtime, which is where
    // a tag written for the web becomes the component that paints it natively.
    const runtimeImport =
      moduleName === "react/jsx-runtime" || moduleName === "react/jsx-dev-runtime";
    if (runtimeImport && !insideBifrost) {
      return { type: "sourceFile", filePath: runtime };
    }
    // A library built before the automatic runtime builds its elements with
    // React.createElement, which the runtime above never sees and which would
    // therefore paint a tag for the web. The module object is patched in place,
    // so every other export still arrives whole.
    if (moduleName === "react" && !insideBifrost) {
      return { type: "sourceFile", filePath: react };
    }
    for (const alias of aliases) {
      const under = underAlias(alias, moduleName);
      if (under !== null) {
        return inherited(context, path.join(alias.to, under + alias.after), platform);
      }
    }
    return inherited(context, moduleName, platform);
  };

  return config;
}

// The compiler throws away what it could not express, and a Tailwind class that
// compiles to nothing looks exactly like one that works: the measured case was
// h-[100dvh], which is no height at all on a native target and a viewport on
// the web. This puts a transformer in front of whatever the app has and reports
// what was dropped, so the next one is a line in the log instead of an
// afternoon. It goes last, outermost, because it has to see the config the
// other wrappers produced.
function withBifrostWarnings(config) {
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    require.resolve("react-native-css/compiler");
  } catch {
    // No stylesheet compiler in the app, so there is nothing it could drop.
    return config;
  }
  config.transformer = {
    ...config.transformer,
    bifrostUpstreamTransformer: config.transformerPath ?? null,
  };
  config.transformerPath = path.join(__dirname, "css-warnings.js");
  return config;
}

module.exports = { withBifrost, withBifrostWarnings, pathAliases };
`

// Runs in the transformer, in place of whatever react-native-css installed, and
// hands every file to it unchanged. What it adds is a second look at a
// stylesheet, because the compiler collects the values it cannot express and the
// transformer never asks for them.
const nativeCSSWarnings = `const path = require("node:path");
const { unstable_transformerPath } = require("@expo/metro-config");
const { compile } = require("react-native-css/compiler");

const worker = require(unstable_transformerPath);

const reported = new Set();

const isStylesheet = (filePath) => /\.(s?css|sass)$/.test(filePath);

// A class whose declaration a native target cannot express compiles to nothing:
// h-svh is a height on the web and no height at all here, and nothing anywhere
// says so -- the measured case was a whole tree that closed to zero height, and
// it took an afternoon. The compiler collects the values it dropped and the
// transformer never asks for them, so this reads them back and names the class
// that asked for each one. A property the target lacks outright -- cursor,
// resize, snap -- is not reported: in every case measured it was cosmetic, and
// the reset Tailwind ships is full of them.
function dropped(css, warnings) {
  const values = warnings.values ?? {};
  const found = [];
  let parse;
  try {
    parse = require("postcss").parse;
  } catch {
    // No CSS parser in the app, so the values are said without the class that
    // asked for them. Less useful and still true.
    for (const [property, list] of Object.entries(values)) {
      for (const value of list) {
        found.push({ at: "", property, value: String(value) });
      }
    }
    return found;
  }
  parse(css).walkRules((rule) => {
    for (const selector of rule.selectors) {
      if (selector.charAt(0) !== "." || selector.includes(" ") || selector.includes("::")) {
        continue;
      }
      rule.walkDecls((declaration) => {
        for (const value of values[declaration.prop] ?? []) {
          if (declaration.value.includes(String(value))) {
            found.push({ at: className(selector), property: declaration.prop, value: declaration.value.trim() });
          }
        }
      });
    }
  });
  return found;
}

// A class reaches the stylesheet escaped: the one the app wrote as h-[100dvh]
// is a selector named .h-\[100dvh\], and a warning that says so cannot be
// searched for. Tailwind escapes what a class name cannot hold with a backslash,
// and what it cannot spell in one character as a hex escape.
function className(selector) {
  return selector.replace(/\\([0-9a-fA-F]{1,6}) ?|\\(.)/g, (_, hex, one) =>
    hex === undefined ? one : String.fromCodePoint(parseInt(hex, 16)),
  );
}

async function report(config, projectRoot, filePath, data, options) {
  let css;
  try {
    const web = await worker.transform(config, projectRoot, filePath, data, { ...options, platform: "web" });
    css = web.output[0].data.css.code.toString();
  } catch {
    // A stylesheet that does not survive the web pipeline has nothing to say.
    return;
  }
  const where = path.relative(projectRoot, filePath) || filePath;
  const warnings = compile(css, { ...config.reactNativeCSS, filename: filePath, projectRoot }).warnings();
  for (const one of dropped(css, warnings)) {
    const said = "bifrost: " + where + (one.at === "" ? "" : ": " + one.at) + ": " + one.property + ": " + one.value + " has no native counterpart";
    if (reported.has(said)) {
      continue;
    }
    reported.add(said);
    console.warn(said);
  }
}

// Whatever transformer the app installed, handed every file unchanged. What this
// adds is a second look at a stylesheet, which is the only thing the compiler
// does not already do.
//
// Metro does not hand a transformer the config the app wrote: it hands it
// config.transformer on its own, flattened, minus the three keys it keeps for
// itself. So the path this wrapper kept is at the top of the object and never at
// config.transformer, and reading it there found nothing and fell back to the
// default transformer -- which, for a stylesheet on a native target, is an empty
// module. The whole stylesheet went out and nothing said so: the app painted
// unstyled, which reads exactly like a stylesheet that did not run.
module.exports.transform = async (config, projectRoot, filePath, data, options) => {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const upstream = require(config.bifrostUpstreamTransformer ?? config.transformer?.bifrostUpstreamTransformer ?? unstable_transformerPath);
  const result = await upstream.transform(config, projectRoot, filePath, data, options);
  // The report costs a second pass over the stylesheet -- the web pipeline the
  // native one already ran -- and it is a development diagnostic, so a build
  // does not pay for it. A stylesheet the app did not write is one it cannot
  // fix, and a node_modules tree has hundreds of them.
  const css = options.type !== "asset" && isStylesheet(filePath);
  if (options.dev !== false && options.platform !== "web" && css && !filePath.includes("node_modules")) {
    await report(config, projectRoot, filePath, data, options);
  }
  return result;
};
`

// Replaces the module every `require("react")` gets, so a library that builds
// its elements by hand goes through the tag map too. It patches the module
// object itself and never a copy: a copy holds the own enumerable keys only, and
// a library that imports React as a namespace would get `undefined` for the
// rest.
const nativeReact = `const React = require("react");

const createElement = React.createElement;

// Lazy, like the jsx runtime wrapper: this module is evaluated while
// react-native is still booting, and pulling it in here would land it ahead of
// the polyfills expo installs.
function map(type) {
  if (typeof type !== "string") {
    return type;
  }
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const { hostComponent } = require("./host");
  return hostComponent(type) ?? type;
}

React.createElement = (type, ...rest) => createElement(map(type), ...rest);

module.exports = React;
`

// A page written for the browser asks its server for "/api/..." and the browser
// puts the origin back in front of it. A native client has no origin, so the
// request goes looking for a host that is not the one the app talks to. What the
// browser does to a relative url is the whole of this, and nothing else.
const nativeNet = `import { ORIGIN } from "./origin";

const original = globalThis.fetch;

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === "string") {
    return input;
  }
  if (input instanceof URL) {
    return input.href;
  }
  return input.url;
}

globalThis.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
  const url = urlOf(input);
  if (url.startsWith("/")) {
    return original(ORIGIN + url, init);
  }
  return original(input, init);
};
`

// A native target has no document: react-native draws views, and a page written
// for the browser reaches for the DOM from places the tag map cannot cover. What
// it asks for is a measurement, a media query, or somewhere to keep a
// preference, and none of them has a native counterpart.
//
// It is written and never imported. A page that never touches a document pays
// nothing for it, because a module nothing imports is never bundled, and a page
// that does turns it on with one line in the overlay's `bifrost/setup.ts`:
//
//	import "./dom";
//
// Which is also why it is here rather than in the overlay: it is the same shim
// in every app, and the app that needs it should not have to write it again.
const nativeDOM = `// The smallest object that answers what a page asks of a document, without
// lying about what is on screen. Everything it cannot answer, it answers as
// absent: a query returns null, an attribute is not there, a measurement is
// zero. That is what a page already handles.

// expo-clipboard is the native counterpart of navigator.clipboard, so the shim
// borrows it when the app has it rather than pretending the copy happened.
const clipboard = () => {
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    return require("expo-clipboard");
  } catch {
    return null;
  }
};

const size = () => {
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const { Dimensions } = require("react-native");
    const window = Dimensions.get("window");
    return { width: window.width, height: window.height };
  } catch {
    return { width: 0, height: 0 };
  }
};

const listeners = new Map();

const element = () => ({
  style: { setProperty() {}, removeProperty() {} },
  classList: { add() {}, remove() {}, toggle() {}, contains: () => false },
  setAttribute() {},
  removeAttribute() {},
  getAttribute: () => null,
  appendChild: (child) => child,
  removeChild() {},
  addEventListener() {},
  removeEventListener() {},
  querySelector: () => null,
  querySelectorAll: () => [],
  getBoundingClientRect: () => ({ top: 0, left: 0, right: 0, bottom: 0, width: 0, height: 0, x: 0, y: 0 }),
  focus() {},
  blur() {},
  scrollIntoView() {},
  closest: () => null,
});

const root = element();

const on = (type, listener) => {
  if (!listeners.has(type)) {
    listeners.set(type, new Set());
  }
  listeners.get(type).add(listener);
};

const off = (type, listener) => {
  listeners.get(type)?.delete(listener);
};

const query = {
  matches: false,
  media: "",
  onchange: null,
  addEventListener() {},
  removeEventListener() {},
  addListener() {},
  removeListener() {},
};

// base-ui reaches a node's window through element.ownerDocument.defaultView,
// so the shim needs a window there; what it asks of it is a computed style, and
// the answer is the empty one.
const computed = {
  getPropertyValue: () => "",
  getPropertyPriority: () => "",
  length: 0,
  item: () => "",
};

const store = new Map();
const { width, height } = size();

// A page measures with ResizeObserver where the browser would tell it when a box
// changes size. Native has no such event, so this answers once, when the
// observation starts, with whatever the node says about itself right then, and
// then stays quiet. It is a partial answer and it is written down as one: a box
// that grows later will not report it.
class ResizeObserver {
  constructor(callback) {
    this.callback = callback;
    this.observed = new Set();
  }
  observe(node) {
    this.observed.add(node);
    const rect = node?.getBoundingClientRect?.() ?? { top: 0, left: 0, width: 0, height: 0 };
    this.callback(
      [
        {
          target: node,
          contentRect: rect,
          borderBoxSize: [],
          contentBoxSize: [],
          devicePixelContentBoxSize: [],
        },
      ],
      this,
    );
  }
  unobserve(node) {
    this.observed.delete(node);
  }
  disconnect() {
    this.observed.clear();
  }
}

const install = () => {
  const target = globalThis;
  target.window ??= target;
  target.getComputedStyle ??= () => computed;
  target.document ??= {
    documentElement: root,
    body: root,
    head: root,
    defaultView: target,
    title: "",
    // There is no document to hold a cookie here, so a read answers that there
    // is none. A session that has to survive belongs in a store of the app's
    // own; the client already sends its cookie jar with every request.
    // A read answers that there is none, and a getter is not a value being
    // handed out.
    get cookie() {
      return String();
    },
    visibilityState: "visible",
    createElement: () => element(),
    createElementNS: () => element(),
    createTextNode: () => ({}),
    getElementById: () => null,
    querySelector: () => null,
    querySelectorAll: () => [],
    addEventListener: on,
    removeEventListener: off,
    dispatchEvent: () => true,
  };
  target.matchMedia ??= () => query;
  target.ResizeObserver ??= ResizeObserver;
  target.Node ??= Object.assign(class Node {}, {
    ELEMENT_NODE: 1,
    DOCUMENT_POSITION_DISCONNECTED: 1,
    DOCUMENT_POSITION_PRECEDING: 2,
    DOCUMENT_POSITION_FOLLOWING: 4,
    DOCUMENT_POSITION_CONTAINS: 8,
    DOCUMENT_POSITION_CONTAINED_BY: 16,
    DOCUMENT_POSITION_IMPLEMENTATION_SPECIFIC: 32,
  });
  target.addEventListener ??= on;
  target.removeEventListener ??= off;
  target.dispatchEvent ??= () => true;
  target.innerWidth ??= width;
  target.innerHeight ??= height;
  target.localStorage ??= {
    getItem: (key) => (store.has(key) ? store.get(key) : null),
    setItem: (key, value) => { store.set(key, String(value)); },
    removeItem: (key) => { store.delete(key); },
    clear: () => { store.clear(); },
    key: (index) => [...store.keys()][index] ?? null,
    get length() { return store.size; },
  };
  if (target.navigator && !target.navigator.clipboard) {
    const api = clipboard();
    target.navigator.clipboard = {
      writeText: async (text) => { await api?.setStringAsync(String(text)); },
      readText: async () => (await api?.getStringAsync()) ?? "",
    };
  }
};

install();

export default install;
`

// The shell an app does not have to write, and the seam it needs to add a line
// to it. Two files an overlay may ship are imported here instead of making the
// app replace the whole file to add one line: the global stylesheet, because the
// utilities come from a stylesheet the app owns, and a module of its own, because
// a native target usually has something to install before the first screen
// renders. Both come before the network shim, which wraps whatever fetch the app
// leaves in place.
func nativeLayout(overlay string) string {
	var imports strings.Builder
	if nativeOverlayHas(overlay, "global.css") {
		imports.WriteString("import \"../global.css\";\n")
	}
	if setup := nativeOverlayModule(overlay, "bifrost/setup"); setup != "" {
		fmt.Fprintf(&imports, "import \"../%s\";\n", setup)
	}
	imports.WriteString("import \"../bifrost/net\";\n\n")
	return imports.String() + nativeLayoutBody
}

// A module an overlay ships, under whichever extension Metro resolves it by.
// The import leaves the extension off: a tsconfig that has not asked for
// allowImportingTsExtensions rejects one that spells it out.
func nativeOverlayModule(overlay, name string) string {
	for _, extension := range []string{".ts", ".tsx", ".js", ".jsx"} {
		if nativeOverlayHas(overlay, name+extension) {
			return name
		}
	}
	return ""
}

func nativeOverlayHas(overlay, name string) bool {
	info, err := os.Stat(filepath.Join(overlay, filepath.FromSlash(name)))
	return err == nil && !info.IsDir()
}

const nativeLayoutBody = `import { Stack } from "expo-router";
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
  // How many error.tsx views the route has above it. The view the generator
  // wrote picks one by __bifrostErrorLevel, counting down from this.
  errorFallbacks: number;
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
import { Component, useEffect, useState, type ReactNode } from "react";
import { ActivityIndicator, StyleSheet, Text, View } from "react-native";
import { loadPage, type PageData } from "./data";
import { onRefresh } from "./refresh";

type State =
  | { status: "loading" }
  | { status: "ready"; data: PageData }
  | { status: "failed"; message: string };

// The props to render with when the page did not survive its own render: the
// error.tsx the generator put at this level, and what it should be told.
type Fallback = { level: number; message: string };

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
  const [fallback, setFallback] = useState<Fallback | null>(null);
  const [nonce, setNonce] = useState(0);
  const href = pathname + query(search, params);

  useEffect(() => onRefresh(() => { setNonce((value) => value + 1); }), []);

  useEffect(() => {
    if (!focused) {
      return;
    }
    let live = true;
    setState({ status: "loading" });
    setFallback(null);
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

  // Each state carries a key, because two of them are a View at the root of this
  // component: without it React keeps the instance and a class arrives after the
  // first render, which reads as the theme changing under the tree and remounts
  // everything below it.
  if (state.status === "loading") {
    return <View key="loading" style={styles.center}><ActivityIndicator /></View>;
  }
  if (state.status === "failed") {
    return <View key="failed" style={styles.center}><Text style={styles.error}>{state.message}</Text></View>;
  }

  const data = state.data;
  const document = data.document ?? ({} as PageData["document"]);

  // A page that throws falls into its nearest error.tsx, and the generator
  // already wrote that branch into the view. What the view is missing is which
  // level to show, and the server that would walk that ladder is not the one
  // rendering. So the boundary walks it here: the deepest error view first, one
  // level up on every further throw, and the message when there is nowhere left
  // to catch it. The message is the one the page threw, because it was thrown
  // here and not on a server that would have replaced it with a status text.
  const onError = (error: unknown) => {
    const message = error instanceof Error ? error.message : String(error);
    setFallback((current) =>
      current === null
        ? { level: (data.errorFallbacks ?? 0) - 1, message }
        : { level: current.level - 1, message },
    );
  };

  if (fallback !== null && fallback.level < 0) {
    return <View key="uncaught" style={styles.center}><Text style={styles.error}>{fallback.message}</Text></View>;
  }

  const props: Record<string, unknown> = fallback === null
    ? data.props
    : { ...data.props, __bifrostError: fallback.message, __bifrostErrorLevel: fallback.level };
  const pageKey = typeof props.pathname === "string" ? props.pathname : pathname;
  const content = (
    <Boundary key={fallback === null ? "page" : "error-" + fallback.level} onError={onError}>
      {render(props, pageKey)}
    </Boundary>
  );

  // The document attributes travel with the page the way the root element of a
  // document carries them. A class is the theme the whole tree reads, and the
  // direction is the writing direction of the tree; a language has no native
  // counterpart, because a text node here inherits no language to read from.
  const direction = document.dir === "rtl" || document.dir === "ltr" ? document.dir : undefined;
  if (document.class === undefined && direction === undefined) {
    return content;
  }
  return <View className={document.class} style={direction === undefined ? undefined : { direction }}>{content}</View>;
}

// The boundary hands the failure up instead of painting it: what replaces the
// page is a decision of the screen, which is the only one that knows how many
// error views the route has and which one comes next.
class Boundary extends Component<{ onError: (error: unknown) => void; children?: ReactNode }, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidCatch(error: unknown) {
    this.props.onError(error);
  }

  render() {
    return <>{this.state.failed ? null : this.props.children}</>;
  }
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
// runtime and the tests read the same list, and it is also the file an app is
// meant to extend: an app always has a tag the generator does not know. Replacing
// the whole file is what an app had to do, and it costs whatever the generator
// adds next. Measured: an app replaced it once, the file later grew a function
// the runtime calls, the replaced copy did not have it, and every element of the
// app threw in development. So the table takes what the app ships on top of it
// and the rest of the file stays whole.
func nativeHost(overlay string) string {
	if nativeOverlayModule(overlay, "bifrost/host-extra") == "" {
		return nativeHostHead + nativeHostBody
	}
	return nativeHostHead + nativeHostExtraImport + nativeHostBody + nativeHostExtraMerge
}

// What the app ships on top of the table: a tag the generator does not know, or
// one it paints its own way. It is merged last, so it wins.
const nativeHostExtraImport = `import { extraTags } from "./host-extra";
`

const nativeHostExtraMerge = `
// The app's own tags, on top: a name the table already has is replaced, and one
// it does not is added.
for (const [name, component] of Object.entries(extraTags)) {
  NAMED[name] = component;
}
`

const nativeHostHead = `import { Children, isValidElement, type ReactNode } from "react";
import { Image, Linking, Pressable, Text, TextInput, View } from "react-native";
import { navigate } from "virtual:bifrost/navigation";
`

const nativeHostBody = `
// Blocks. They hold other blocks, which a View can.
const BOXES = ["address", "article", "aside", "caption", "dd", "details", "dialog", "div", "dl", "dt", "fieldset", "figcaption", "figure", "footer", "form", "header", "legend", "li", "main", "nav", "ol", "output", "section", "summary", "table", "tbody", "td", "tfoot", "th", "thead", "tr", "ul"];

// Everything whose content is text.
const TEXTS = ["abbr", "b", "br", "cite", "code", "em", "h1", "h2", "h3", "h4", "h5", "h6", "i", "label", "mark", "p", "pre", "q", "s", "small", "span", "strong", "time", "u"];

// Sheet links, metadata and scripts have nothing to paint.
const HIDDEN = ["base", "head", "link", "meta", "noscript", "script", "style", "title"];

// Props whose whole meaning is the document: a page writes them for the browser,
// a native view has nothing to do with them, and the renderer drops them without
// a word. Two cases earn a warning. One is content that disappears -- a
// dangerouslySetInnerHTML paragraph paints nothing at all. The other is a
// surface the target does not have -- contentEditable is dead text here, and
// nothing says the editing went away. Both are invisible until somebody
// compares the two targets, so the runtime names them in development. A prop the
// web gets for free and native does not -- srcset, loading -- is not worth a
// line, because leaving it out changes nothing. Measured: htmlFor is not either.
// It is an association a reader still reads through, and fonnso alone writes it
// twenty-nine times, which would be twenty-nine warnings for a paragraph still
// on the screen.
const DOM_ONLY = ["dangerouslySetInnerHTML", "contentEditable"];

// The props of an element that a native target cannot honour. Called by the
// runtime, which is the only place that still knows the tag the page wrote.
// A name that is not all lowercase is not a tag: react-native makes its own
// views out of strings like "RCTView", and the prop the page leaked travels
// down to them, so answering for those would say the same thing twice.
export function droppedProps(name: string, props: unknown): string[] {
  if (name !== name.toLowerCase() || props === null || typeof props !== "object") {
    return [];
  }
  return DOM_ONLY.filter((prop) => (props as Record<string, unknown>)[prop] !== undefined);
}

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
// native views apart. A name that is missing says so, because the alternative
// is a paragraph that quietly is not there.
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
type Host = {
  hostComponent(name: string): unknown;
  droppedProps(name: string, props: unknown): string[];
};

let host: Host | null = null;

function loaded(): Host {
  if (host === null) {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    host = require("./host") as Host;
  }
  return host;
}

function component(name: string): unknown {
  const mapped = loaded().hostComponent(name);
  return mapped === undefined ? name : mapped;
}

// A prop the target cannot honour would otherwise be dropped in silence, and a
// page would look right while missing a paragraph. This is the last place that
// knows both the tag the page wrote and the props it was given.
function warnDropped(name: string, props: unknown) {
  for (const prop of loaded().droppedProps(name, props)) {
    console.warn("bifrost: a native target drops " + prop + " on <" + name + ">");
  }
}

// A tag a page wrote for the web becomes the component that paints it natively;
// a name this table does not know is left alone, because react-native builds
// its own views out of strings too.
const wrap = (create: unknown) =>
  (type: unknown, ...rest: unknown[]) => {
    if (__DEV__ && typeof type === "string") {
      warnDropped(type, rest[0]);
    }
    return (create as (type: unknown, ...rest: unknown[]) => unknown)(
      typeof type === "string" ? component(type) : type,
      ...rest,
    );
  };

export const jsx = wrap(runtime.jsx);
export const jsxs = wrap(runtime.jsxs);
export const jsxDEV = wrap(runtime.jsxDEV);
export const Fragment = runtime.Fragment;
`

var nativeProjectFiles = []nativeFile{
	{"tsconfig.json", nativeTSConfig},
	{"metro.config.js", nativeMetroConfig},
	{"bifrost/css-warnings.js", nativeCSSWarnings},
	{"bifrost/data.ts", nativeData},
	{"bifrost/dom.js", nativeDOM},
	{"bifrost/jsx-runtime.ts", nativeJSXRuntime},
	{"bifrost/metro.js", nativeMetroHelper},
	{"bifrost/net.ts", nativeNet},
	{"bifrost/react.js", nativeReact},
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
		if warning := nativeOverlayWarning(overlay); warning != "" {
			_, _ = fmt.Fprintln(os.Stderr, warning)
		}
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
	// The layout and the tag map are written with the overlay in hand, because
	// what they contain depends on what the app ships next to them.
	for name, content := range map[string]string{
		"app/_layout.tsx":  nativeLayout(overlay),
		"bifrost/host.tsx": nativeHost(overlay),
	} {
		if err := writeNativeFile(native, name, content, 0o644); err != nil {
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
	previous, err := readOverlayManifest(native)
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
	if err := removeStaleOverlayFiles(native, previous, names); err != nil {
		return err
	}
	return os.WriteFile(overlayManifest(native), []byte(manifestOf(names)), 0o644)
}

// An app deletes a screen, renames a module, drops a directory, and what was
// copied the last time stays in the project for good: the generator only clears
// `app/` and `bifrost/`, so anything the overlay put beside them is never looked
// at again. Measured: a file removed from the overlay was still there two
// regenerations later. Nothing in the generated project is worth keeping -- it
// is a build artifact and every run rewrites it -- so what was copied is written
// down and the next run removes what the overlay no longer ships.
func removeStaleOverlayFiles(native string, previous, current []string) error {
	kept := make(map[string]bool, len(current))
	for _, name := range current {
		kept[name] = true
	}
	for _, name := range previous {
		if kept[name] || generatedByBifrost(name) {
			continue
		}
		if err := os.Remove(filepath.Join(native, filepath.FromSlash(name))); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// The two directories every run rewrites, and the manifest the generator owns.
// A file there is not the overlay's to remove, and an overlay that stops
// restyling a screen leaves the generated one in its place.
func generatedByBifrost(name string) bool {
	for _, prefix := range []string{"app/", "bifrost/"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return name == "package.json"
}

// The manifest sits next to the project and not inside it, because everything
// inside it is a build artifact and a build artifact is what this is about.
const overlayManifestName = "native-overlay.json"

func overlayManifest(native string) string {
	return filepath.Join(filepath.Dir(native), overlayManifestName)
}

func manifestOf(names []string) string {
	content, err := json.Marshal(names)
	if err != nil {
		return "[]\n"
	}
	return string(content) + "\n"
}

func readOverlayManifest(native string) ([]string, error) {
	content, err := os.ReadFile(overlayManifest(native))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	if err := json.Unmarshal(content, &names); err != nil {
		// A manifest this cannot read is one from another version, and the worst
		// it can do is leave a file behind, which is where it started.
		return nil, nil
	}
	return names, nil
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

// An app that ships a metro.config.js of its own keeps that file whole and
// loses everything the generated one installed. Every seam but one fails loud:
// a missing resolver leaves a module unresolved. The CSS diagnostics fail
// quietly, because not being installed and having nothing to report look
// exactly the same -- the transformer is simply never wrapped, and every class
// the target cannot express goes back to being invisible. The generated config
// is the only place that installs it, so the generator reads the file it is
// about to leave alone and says what has to be there.
func nativeOverlayWarning(overlay string) string {
	name := filepath.Join(overlay, "metro.config.js")
	content, err := os.ReadFile(name)
	if err != nil {
		return ""
	}
	if strings.Contains(string(content), "withBifrostWarnings") {
		return ""
	}
	return fmt.Sprintf(
		"bifrost: %s replaces the generated metro.config.js, so it has to call withBifrostWarnings outermost:\n"+
			"  module.exports = withBifrostWarnings(withBifrost(yourConfig));\n"+
			"  without it, a class the native target cannot express is dropped without a word.",
		filepath.ToSlash(name),
	)
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
		if base == nil {
			base = map[string]any{}
		}
		for name, version := range added {
			base[name] = version
		}
		manifest[key] = base
	}
	// A dependency that is added is not necessarily the one that gets installed:
	// the generator's own packages ask for ranges that a newer release also
	// satisfies, and the package manager is free to hoist that one instead. An
	// `overrides` block is the only way an app can say which copy wins, so it is
	// merged like the dependencies are.
	if declared["overrides"] != nil {
		manifest["overrides"] = declared["overrides"]
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
