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
    "strict": true,
    "allowImportingTsExtensions": true,
    "paths": {
      "virtual:bifrost/navigation": ["./bifrost/navigation"]
    }
  }
}
`

// Metro resolves the virtual module and a stylesheet; tsc does not, and both are
// written by this generator or asked for by the layout it writes.
const nativeTypes = `declare module "*.css";
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
// An app written for the NodeNext rules spells an import with the extension the
// file will have once compiled -- "./dayjs.js" for dayjs.ts -- and the web
// bundler reads the TypeScript file behind it. Metro looks for the .js file that
// is not there and stops. Measured on the app of the case: the whole bundle
// stopped at the first of two hundred imports written that way.
function typescriptOf(moduleName) {
  if (!moduleName.endsWith(".js")) {
    return [];
  }
  const base = moduleName.slice(0, -3);
  return [base + ".ts", base + ".tsx"];
}

function resolveWithTypescript(inherited, context, moduleName, platform) {
  try {
    return inherited(context, moduleName, platform);
  } catch (error) {
    for (const one of typescriptOf(moduleName)) {
      try {
        return inherited(context, one, platform);
      } catch {
        // The next candidate, and the error of the .js file at the end.
      }
    }
    throw error;
  }
}

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

  // react-dom is patched where it is loaded -- one portal that renders inline
  // instead of into a document that does not exist, see bifrost/dom.js -- and the
  // patch is one patch only if every module is looking at the same copy of it.
  // Measured: the bundle of the case carried two, one reached from the app's own
  // components (at the workspace root) and one from this project, and the page
  // was calling the unpatched one -- the menu of a tab answered "Target container
  // is not a DOM element", which in a release build reaches the screen as a
  // minified React error with nothing to read. The name is resolved once, from
  // the workspace, for every module that asks for it; asking from there is what
  // makes the copies one.
  const reactDOMFrom = (context, platform, name) => {
    try {
      return inherited(
        { ...context, originModulePath: path.join(workspaceRoot, "package.json") },
        name,
        platform,
      );
    } catch {
      return inherited(context, name, platform);
    }
  };

  config.resolver.resolveRequest = (context, moduleName, platform) => {
    if (moduleName === "react-dom") {
      return reactDOMFrom(context, platform, moduleName);
    }
    if (moduleName === "virtual:bifrost/navigation") {
      return { type: "sourceFile", filePath: navigation };
    }
    // The wrapper needs the real one, so its own import is left alone, and so is
    // every import inside the generated project.
    const insideBifrost = context.originModulePath.startsWith(bifrost);
    // The one collection of styles reads this module, and a flex container has
    // to mean a row before a rule is merged with another.
    if (moduleName === "react-native-css/native-internal" && !insideBifrost) {
      return { type: "sourceFile", filePath: path.join(bifrost, "native-internal.js") };
    }
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
        return resolveWithTypescript(inherited, context, path.join(alias.to, under + alias.after), platform);
      }
    }
    return resolveWithTypescript(inherited, context, moduleName, platform);
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

module.exports = { withBifrost, withBifrostWarnings, pathAliases, resolveWithTypescript, typescriptOf };
`

// A browser lays a flex container out along the row; a native view lays its
// children down the page, so a `display: flex` on its own means a column here.
// Tailwind's `.flex` and `.inline-flex` are that display and nothing else, and
// with them every row an app writes comes out a column -- measured on device:
// two boxes of `h-4 w-8` under `flex` stacked one above the other, and the same
// two under `flex-row` went side by side. The direction goes into the rule that
// asked for the flex, and not into a rule of its own, because the runtime
// settles two rules by their place in the stylesheet: a rule of its own would
// land after the `.flex-col` Tailwind emits and win. Inside the rule the later
// `.flex-col` still wins, which is what the browser does, and a rule that
// already names a direction is left alone.
const nativeRowDirection = `// The display of a flex container is a row in the browser, and a native view is
// a column until it is told otherwise. This writes the direction the browser
// would have used into the rule that asked for the flex, and leaves a rule that
// already names one untouched -- so running this twice is running it once.

const FLEX = /^(inline-)?flex$/i;

// A declaration whose value is a variable is written as an array, and one the
// compiler resolved is an object. Only the second can be read here, and a
// direction that arrives through a variable is left to the variable.
function names(declarations, property) {
  let found = false;
  for (const group of declarations) {
    if (Array.isArray(group) || !group) {
      continue;
    }
    if (Object.prototype.hasOwnProperty.call(group, property)) {
      found = true;
    }
  }
  return found;
}

function withRowDirection(stylesheet) {
  for (const entry of (stylesheet && stylesheet.s) || []) {
    for (const rule of entry[1] || []) {
      const declarations = rule && rule.d;
      if (!Array.isArray(declarations) || names(declarations, "flexDirection")) {
        continue;
      }
      for (const group of declarations) {
        if (Array.isArray(group) || !group) {
          continue;
        }
        if (typeof group.display === "string" && FLEX.test(group.display.trim())) {
          group.flexDirection = "row";
          break;
        }
      }
    }
  }
  return stylesheet;
}

module.exports = { withRowDirection };
`

// A browser hands the typeface, the size, the weight and the colour of a box to
// every word inside it. A native view hands nothing: react-native passes text
// styles from a Text to the Text inside it and stops at a View, so a size
// written on a wrapper reaches the browser and no screen. The properties a
// browser inherits are collected on the way down here, and folded in where the
// words are.
//
// The other half is the typeface. A native screen picks a font by family and
// loads it as a file, so an app that ships one file per weight gets a family of
// its own for each one, and the weight a rule asked for has to become part of
// the name it looks for. The files are declared by the app and loaded by the
// generated layout; see ./fonts.
const nativeTextStyle = `// A browser hands these down, so a box keeps them for what it holds. Only these
// keys travel: a margin is the box's own and is not inherited anywhere.
const INHERITED = ["color", "fontFamily", "fontSize", "fontStyle", "fontWeight", "letterSpacing", "lineHeight", "textAlign", "textTransform"];

// The two weights a stylesheet may spell, and the number a font file is named
// with.
const NAMED = { normal: 400, bold: 700 };

function weightOf(value) {
  if (typeof value === "number") {
    return value > 0 ? value : null;
  }
  if (typeof value !== "string") {
    return null;
  }
  const said = value.trim().toLowerCase();
  if (said === "") {
    return null;
  }
  if (NAMED[said]) {
    return NAMED[said];
  }
  const number = Number(said);
  return Number.isFinite(number) && number > 0 ? number : null;
}

// The weight of the files the app ships that is closest to the one asked for,
// which is how a browser matches a weight it has no file for. A tie goes to the
// heavier one.
function nearest(weights, want) {
  let best = weights[0];
  for (const weight of weights) {
    const distance = Math.abs(weight - want);
    const winning = Math.abs(best - want);
    if (distance < winning || (distance === winning && weight > best)) {
      best = weight;
    }
  }
  return best;
}

// What a box hands to what it holds: the text properties of its own style, over
// the ones it was handed.
function inheritedText(carried, style) {
  const out = { ...(carried || {}) };
  for (const one of Array.isArray(style) ? style : [style]) {
    if (!one || typeof one !== "object") {
      continue;
    }
    for (const key of INHERITED) {
      if (one[key] !== undefined) {
        out[key] = one[key];
      }
    }
  }
  return out;
}

// The family the app declared, by the name a stylesheet wrote. The compiler
// writes one as a single token -- measured, 'DejaVu Serif' reaches the style as
// 'DejaVuSerif' -- and a family of the web is case-insensitive, so the name is
// compared with its spaces and its case folded away. What comes back is the
// declared spelling, which is the one the layout loaded.
function declaredFamily(name, families) {
  if (typeof name !== "string") {
    return null;
  }
  const said = name.replace(/\s+/g, "").toLowerCase();
  for (const family of Object.keys(families || {})) {
    if (family.replace(/\s+/g, "").toLowerCase() === said) {
      return family;
    }
  }
  return null;
}

// The text properties of one element, with the family it names turned into the
// file that carries the weight it asks for. A family the app does not ship, or a
// weight that arrives as something else, is left exactly as it was, and a
// browser's initial weight is 400.
function textStyle(carried, families) {
  const out = { ...(carried || {}) };
  const family = declaredFamily(out.fontFamily, families);
  const weights = family === null ? undefined : families[family];
  if (!Array.isArray(weights) || weights.length === 0) {
    return out;
  }
  const weight = weightOf(out.fontWeight);
  out.fontFamily = family + " " + nearest(weights, weight === null ? 400 : weight);
  delete out.fontWeight;
  return out;
}

module.exports = { inheritedText, textStyle };
`

// The components the tag map paints with, wrapped so that what a browser
// inherits keeps travelling down the tree and a weight reaches the file that has
// it. Each wrapper is styled by the style runtime first -- that is the only
// place where the rules of an element are already merged into one style -- and
// then hands that style on: a Text folds what it carries into what it paints,
// and a box keeps what the words inside it should inherit.
const nativeTextModule = `import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { Animated, Pressable, Text, TextInput, View, type StyleProp, type ViewStyle } from "react-native";
import { styled } from "react-native-css/native";
import * as declaredFonts from "./fonts";
import { useDocumentScroll } from "./document-scroll";
import { inheritedText, textStyle } from "./text-style";
import { placed } from "./place";

// The weights of each family the app ships, if it ships any: a project that
// keeps a module of its own here declares no families, and every name is left
// exactly as the app wrote it.
const families = (declaredFonts.families ?? {}) as Record<string, number[]>;

// What a box hands to the text inside it. Empty until a box sets something, so
// an app that inherits nothing pays a context read and no style.
const Carried = createContext<Record<string, unknown>>({});

type HostProps = { style?: unknown; children?: ReactNode } & Record<string, unknown>;

// The class of an element is the style it resolved to, and handing it on would
// resolve it a second time.
function withoutClass(rest: Record<string, unknown>) {
  const painted = { ...rest };
  delete painted.className;
  return painted;
}

function TextView({ style, children, ...rest }: HostProps) {
  const carried = useContext(Carried);
  const own = inheritedText(carried, style);
  return (
    <Carried.Provider value={own}>
      <Text {...withoutClass(rest)} style={[placed(style), textStyle(own, families)]}>{children}</Text>
    </Carried.Provider>
  );
}

function ViewBox({ style, children, ...rest }: HostProps) {
  const carried = useContext(Carried);
  return (
    <Carried.Provider value={inheritedText(carried, style)}>
      <View {...withoutClass(rest)} style={placed(style) as StyleProp<ViewStyle>}>{children}</View>
    </Carried.Provider>
  );
}

function AnimatedViewBox({ style, bottomAnchored, children, ...rest }: HostProps) {
  const carried = useContext(Carried);
  const document = useDocumentScroll();
  const anchored = document !== null && document.viewport > 0 && bottomAnchored === true;
  const correction = document === null || !anchored ? 0 : document.viewport - document.content;
  const painted = document === null ? placed(style) : [placed(style), { transform: [{ translateY: Animated.add(document.y, correction) }] }];
  return (
    <Carried.Provider value={inheritedText(carried, style)}>
      <Animated.View {...withoutClass(rest)} style={painted as StyleProp<ViewStyle>}>{children}</Animated.View>
    </Carried.Provider>
  );
}

type LayoutNode = {
  measureLayout: (relative: unknown, onSuccess: (x: number, y: number, width: number, height: number) => void, onFail?: () => void) => void;
};

// The box a page stuck to the edge of the document: it starts where the page put
// it and, once the content has scrolled that far, travels with the content by as
// much as it scrolls -- the box stays at the edge while the content goes under
// it. The distance from the top of the document is measured once, against the
// box the document scrolls, and measured again whenever the content changes size.
function StickyViewBox({ style, children, ...rest }: HostProps) {
  const carried = useContext(Carried);
  const document = useDocumentScroll();
  const view = useRef<LayoutNode | null>(null);
  const [top, setTop] = useState<number | null>(null);
  const content = document === null ? 0 : document.content;
  useEffect(() => {
    const node = view.current;
    const inner = document === null ? null : document.inner;
    if (node === null || inner === null || inner.current === null) {
      return;
    }
    node.measureLayout(inner.current, (x: number, y: number) => { setTop(y); }, () => { setTop(null); });
  }, [document, content]);
  const shift = document === null || top === null
    ? null
    : top <= 0
      ? document.y
      : document.y.interpolate({ inputRange: [-1, 0, top, top + 1, 1e9], outputRange: [0, 0, 0, 1, 1e9] });
  const painted = shift === null ? placed(style) : [placed(style), { transform: [{ translateY: shift }] }];
  return (
    <Carried.Provider value={inheritedText(carried, style)}>
      <Animated.View ref={view as never} {...withoutClass(rest)} style={painted as StyleProp<ViewStyle>}>{children}</Animated.View>
    </Carried.Provider>
  );
}

// A field of the web is a box the page sized, and the text of a native input sits
// in the middle of its line with the padding of the font around it: a field with
// the height of the page clipped the top of what was typed. Measured on the login
// of the app of the case, the address was cut in half.
const INPUT_FIT = { includeFontPadding: false, textAlignVertical: "center" as const, paddingVertical: 0 };

function InputBox({ style, children, ...rest }: HostProps) {
  const carried = useContext(Carried);
  const own = inheritedText(carried, style);
  // The line height a rule of the page asked for is the line of a browser, and a
  // native input keeps its own: with both, the line is taller than the box the
  // page sized and the top of what was typed is cut. Measured on the login of the
  // app of the case.
  const text = { ...(textStyle(own, families) as Record<string, unknown>) };
  delete text.lineHeight;
  return (
    <Carried.Provider value={own}>
      <TextInput {...withoutClass(rest)} style={[placed(style), text, INPUT_FIT]}>{children}</TextInput>
    </Carried.Provider>
  );
}

function PressBox({ style, children, ...rest }: HostProps) {
  const carried = useContext(Carried);
  return (
    <Carried.Provider value={inheritedText(carried, style)}>
      <Pressable {...withoutClass(rest)} style={placed(style) as StyleProp<ViewStyle>}>{children}</Pressable>
    </Carried.Provider>
  );
}

export const StyledText = styled(TextView);
export const StyledView = styled(ViewBox);
export const StyledAnimatedView = styled(AnimatedViewBox);
export const StyledStickyView = styled(StickyViewBox);
export const StyledPressable = styled(PressBox);
export const StyledTextInput = styled(InputBox);

// The colour a box hands down, for the one element that paints it without being
// a text: a drawing of the web takes currentColor from the box that holds it.
export function useInheritedColor(): string | undefined {
  const carried = useContext(Carried);
  return typeof carried.color === "string" ? carried.color : undefined;
}
`

// A flex container means a row to a browser and a column to a native view, and
// the collection of styles is the last place where a rule still says what it
// asked for, before another rule for the same element is merged into it. The
// module is patched where it is read and never copied: the app's generated
// stylesheets and the library's own modules hold this same collection.
const nativeNativeInternal = `// The runtime keeps one collection of styles for the whole app, and everything
// that reads it reads this one. Every stylesheet the compiler produces is handed
// in here, which is the only place that sees a rule before it is merged with the
// rules of the same element.
const upstream = require("react-native-css/native-internal");
const { withRowDirection } = require("./row-direction");

const collection = upstream.StyleCollection;
if (collection && typeof collection.inject === "function") {
  const inject = collection.inject.bind(collection);
  collection.inject = (stylesheet) => inject(withRowDirection(stylesheet));
}

module.exports = upstream;
`

// A browser answers a selector that names a tag -- `.md p` is every paragraph of
// the markdown -- and a native view has no tag to name, so the compiler drops
// the rule in silence. Measured on fonnso: of the 24 rules of its markdown
// block, 17 compiled to nothing, and every paragraph and every list came out
// bare, on a screen that was otherwise right.
//
// What a native view has is a class, so the map paints every element it builds
// with a class named after the tag -- native-p, native-li -- and this rewrites
// the element selectors of a stylesheet into those classes. The stylesheet of
// the app needs no change, and the web target never sees any of it.
//
// Two things are said out loud instead, because neither has a counterpart here
// and the compiler drops them the same silent way: a pseudo-element (::marker,
// ::before) and a structural pseudo-class (:first-child, :nth-child). :where()
// is expanded instead, because its alternatives are selectors in their own
// right.
//
// It runs over the stylesheet the pipeline produced, and not over the file the
// app wrote: the rules that name a tag are the ones Tailwind and its plugins
// generate -- the preflight of v4 puts the default typeface of the stylesheet on
// `html`, every reset puts a margin on `p` -- and rewriting the file before the
// directives are expanded left all of them in place and dropped. Measured on a
// stylesheet whose only source was `@import "tailwindcss"`: expanded first, the
// `html, :host` rule becomes the class the screen paints, and the typeface of
// the theme reaches the text.
const nativeSelectors = `// The class an element carries because of the tag it was written as.
function classOf(tag) {
  return "native-" + tag;
}

// The pseudo-classes that name a position in the tree, and the pseudo-elements
// that name a box inside the element. A native view has neither.
const STRUCTURAL = /^:(first|last|only|nth)-child$|^:empty$|^:(first|last|only|nth)-of-type$/;

function parser() {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  return require("postcss-selector-parser");
}

function postcssOf() {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  return require("postcss");
}

// The first :where() of a selector, which is the one that has to come out first.
function firstWhere(selector) {
  let found = null;
  selector.walkPseudos((pseudo) => {
    if (found === null && pseudo.value === ":where" && pseudo.nodes !== undefined && pseudo.nodes.length > 0) {
      found = pseudo;
    }
  });
  return found;
}

// One selector into the selectors it stands for, with every :where() written
// out: :where(p, ul) is two selectors that mean the same rule, and a native
// target has no :where() to hold them together.
function written(selector) {
  const where = firstWhere(selector);
  if (where === null) {
    return [selector];
  }
  const out = [];
  for (const alternative of where.nodes) {
    const copy = selector.clone();
    firstWhere(copy).replaceWith(...alternative.nodes.map((one) => one.clone()));
    out.push(...written(copy));
  }
  return out;
}

// The tags of a selector become the classes the map paints them with, and what
// is left that a native view cannot express is answered to the caller.
// The pseudos whose argument is a selector of its own: a tag inside :not(p) is
// an element and becomes a class, while the "2" of :nth-child(2) and the "ar" of
// :lang(ar) are not elements. Renaming those leaves CSS no compiler can read --
// measured on the stylesheet of the app of the case: :nth-child(2) came out as
// :nth-child(.native-2), and the whole stylesheet stopped with "Unexpected token
// Delim('.')".
const SELECTOR_ARGUMENT = /^:(where|is|not|has|host|host-context|matches|any|-moz-any|-webkit-any|slotted)$/;

function namesAnElement(tag) {
  for (let parent = tag.parent; parent !== undefined && parent !== null; parent = parent.parent) {
    if (parent.type === "pseudo") {
      return SELECTOR_ARGUMENT.test(parent.value);
    }
  }
  return true;
}

function renamed(selector, onLost) {
  selector.walkTags((tag) => {
    if (!namesAnElement(tag)) {
      return;
    }
    tag.replaceWith(parser().className({ value: classOf(tag.value) }));
  });
  selector.walkPseudos((pseudo) => {
    if (pseudo.value.startsWith("::") || STRUCTURAL.test(pseudo.value)) {
      onLost(pseudo.toString());
    } else if ((pseudo.value === ":where" || pseudo.value === ":is") && pseudo.nodes !== undefined) {
      onLost(pseudo.value + "(...)");
    }
  });
}

// Every element selector of a stylesheet, written as the class the element
// carries. A selector with no tag in it comes out as it went in.
function withTagClasses(css, onLost) {
  const text = typeof css === "string" ? css : css.toString("utf8");
  let postcss;
  let selectors;
  try {
    postcss = postcssOf();
    selectors = parser();
  } catch (error) {
    onLost("", "no parser to read a selector with: " + error.message);
    return text;
  }
  let root;
  try {
    root = postcss.parse(text);
  } catch (error) {
    onLost("", "a stylesheet that could not be read: " + error.message);
    return text;
  }
  root.walkRules((rule) => {
    // The steps of an animation are rules with a selector that is not one, and
    // renaming them would take the animation apart.
    const parent = rule.parent;
    if (parent !== undefined && parent.type === "atrule" && parent.name.indexOf("keyframes") >= 0) {
      return;
    }
    const before = rule.selectors;
    const after = [];
    for (const selector of before) {
      let tree;
      try {
        tree = selectors().astSync(selector);
      } catch {
        after.push(selector);
        continue;
      }
      for (const one of written(tree)) {
        renamed(one, (what) => onLost(selector, what));
        after.push(one.toString());
      }
    }
    if (after.join(",") !== before.join(",")) {
      rule.selector = after.join(",");
    }
  });
  return root.toString();
}

module.exports = { withTagClasses, classOf };
`

const nativePosition = `// A box a page pins to the viewport -- position: fixed -- is a box the browser
// paints over everything and never scrolls away. A native view has no viewport to
// pin to; the closest thing it has is a box laid out against the box that holds
// it, which is what a fixed header or a fixed bar is asking for. Measured on the
// app of the case: the header of the shell was a fixed box and the scroll view
// was its sibling, and the header was painted nowhere at all, because the
// compiler drops a position it cannot express and a box with no position stays
// in the flow the scroll view covers.
//
// The rewrite happens before the compiler, because after it the declaration is
// gone: a value the compiler does not know is a value it dropped.
function withFixed(css) {
  const text = typeof css === "string" ? css : css.toString("utf8");
  let postcss;
  try {
    postcss = require("postcss");
  } catch {
    return text;
  }
  let root;
  try {
    root = postcss.parse(text);
  } catch {
    return text;
  }
  root.walkDecls("position", (declaration) => {
    if (declaration.value.trim() === "fixed") {
      declaration.value = "absolute";
    }
  });
  return root.toString();
}

module.exports = { withFixed };
`

// Runs in the transformer, in place of whatever react-native-css installed.
// Every file but a stylesheet is handed on unchanged. A stylesheet is the one
// thing it does itself, because the order of the steps is the whole point: the
// web pipeline expands Tailwind, the selectors that name a tag become classes on
// what comes out, a position the target cannot express becomes the one it can,
// and only then is it compiled for the target. The runtime the
// app installed compiles the stylesheet as it gets it, which is before the
// directives are expanded, so the rules the pipeline generates were never
// rewritten and every one of them was dropped.
const nativeCSSWarnings = `const path = require("node:path");
const { unstable_transformerPath } = require("@expo/metro-config");
const { compile } = require("react-native-css/compiler");
const { withTagClasses } = require("./selectors");
const { withFixed } = require("./position");
const { scrollOf } = require("./scroll");

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
// The keyframes of the web run on the animation module of the target, and the one
// the runtime of this target carries has no interpolator for a transform:
// measured on the first screen of the app, a press that animates stopped the
// render with "[Reanimated] No interpolator factory found for property:
// transform". The target paints without animation, which is what a browser with
// the animations turned off paints.
function withoutAnimations(css) {
  let parse;
  try {
    parse = require("postcss").parse;
  } catch {
    return css;
  }
  const root = parse(css);
  root.walkDecls((declaration) => {
    if (declaration.prop === "animation" || declaration.prop.startsWith("animation-")) {
      declaration.remove();
    }
  });
  root.walkAtRules((at) => {
    if (/^(-\w+-)?keyframes$/.test(at.name)) {
      at.remove();
    }
  });
  return root.toString();
}

function dropped(css, warnings) {
  const values = warnings.values ?? {};
  // A class the map answers with a ScrollView, or with a child of one, is not a
  // class this target dropped, and reporting it would be a line in the log about
  // something that works.
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
      const at = className(selector);
      const name = at.charAt(0) === "." ? at.slice(1) : at;
      rule.walkDecls((declaration) => {
        for (const value of values[declaration.prop] ?? []) {
          if (!declaration.value.includes(String(value))) {
            continue;
          }
          if (scrollOf(name) !== null) {
            continue;
          }
          // A sticky child is pinned by the scroll view that holds it, not by a
          // position: the class works and has no counterpart to warn about.
          if (name === "sticky" && declaration.prop === "position" && String(value) === "sticky") {
            continue;
          }
          found.push({ at, property: declaration.prop, value: declaration.value.trim() });
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

function report(projectRoot, filePath, css, warnings) {
  const where = path.relative(projectRoot, filePath) || filePath;
  for (const one of dropped(css, warnings)) {
    const said = "bifrost: " + where + (one.at === "" ? "" : ": " + one.at) + ": " + one.property + ": " + one.value + " has no native counterpart";
    if (reported.has(said)) {
      continue;
    }
    reported.add(said);
    console.warn(said);
  }
}

// A selector a native target cannot express is dropped by the compiler in
// silence, which is what a selector that works looks like too. This names the
// one the stylesheet has and the reason, and it is a development diagnostic: an
// app cannot fix a stylesheet inside a dependency, and a node_modules tree has
// hundreds of them.
function lostSelectors(config, projectRoot, filePath, options) {
  if (options.dev === false || filePath.includes("node_modules")) {
    return () => {};
  }
  const where = path.relative(projectRoot, filePath) || filePath;
  return (selector, what) => {
    const said =
      "bifrost: " +
      where +
      (selector === "" ? "" : ": " + selector) +
      ": " +
      what +
      " has no native counterpart";
    if (reported.has(said)) {
      return;
    }
    reported.add(said);
    console.warn(said);
  };
}

// The module Metro bundles for a stylesheet on a native target: one that injects
// the compiled sheet into the collection the whole app reads. It is what the
// runtime the app installed writes, spelled here because this file is the one
// that compiles now.
function injected(stylesheet) {
  return Buffer.from(
    'import { StyleCollection } from "react-native-css/native-internal";\n' +
      "StyleCollection.inject(" + JSON.stringify(stylesheet) + ");export {};",
  );
}

// Whatever transformer the app installed, handed every file unchanged. A
// stylesheet is compiled here, and in this order: the web pipeline expands
// Tailwind, the selectors that name a tag become the classes the map paints, and
// the result is what the target compiles. The step that would otherwise be
// missing is the second one, and without it the preflight of the theme and the
// rules of a plugin compile to nothing.
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
  if (options.type === "asset" || options.platform === "web" || !isStylesheet(filePath)) {
    return upstream.transform(config, projectRoot, filePath, data, options);
  }
  const web = await worker.transform(config, projectRoot, filePath, data, { ...options, platform: "web" });
  const produced = web.output[0]?.data?.css?.code;
  const expanded = produced === undefined ? data.toString("utf8") : produced.toString();
  const source = withFixed(withTagClasses(withoutAnimations(expanded), lostSelectors(config, projectRoot, filePath, options)));
  const compiled = compile(source, {
    ...(config.reactNativeCSS ?? options.reactNativeCSS),
    filename: filePath,
    projectRoot,
  });
  // The report is a development diagnostic, so a build does not pay for it. A
  // stylesheet the app did not write is one it cannot fix, and a node_modules
  // tree has hundreds of them.
  if (options.dev !== false && !filePath.includes("node_modules")) {
    report(projectRoot, filePath, source, compiled.warnings());
  }
  const result = await upstream.transform(config, projectRoot, filePath + ".js", injected(compiled.stylesheet()), options);
  // Metro asked this file for its CSS and got a module: the sheet is inside it,
  // and a second copy of it as an asset is a file nothing reads.
  result.output[0].data.css = { skipCache: true, code: "" };
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
//
// A component of the app builds its elements with createElement as often as with
// jsx -- base-ui does it for every primitive it renders -- and the browser's
// name for the interaction of an element has to be answered on that road too.
function mapped(type, props) {
  if (typeof type !== "string") {
    return null;
  }
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const host = require("./host");
  const component = host.hostComponent(type);
  return {
    type: component ?? type,
    props: host.nativeProps(props, component === undefined ? undefined : type),
  };
}

React.createElement = (type, ...rest) => {
  const host = mapped(type, rest[0]);
  if (host === null) {
    return createElement(type, ...rest);
  }
  return createElement(host.type, host.props, ...rest.slice(1));
};

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

// The fetch of the runtime turns every part of a FormData into bytes, and a file
// the page chose is a path of the device and not bytes: a request with one is
// refused as "Unsupported FormDataPart implementation" -- measured on the device
// with a picked document. The XMLHttpRequest of the target is the uploader that
// reads that part, so a multipart request with a file is sent by it.
function isUpload(body: unknown): body is FormData {
  if (!(body instanceof FormData)) {
    return false;
  }
  for (const [, value] of body.entries()) {
    if (typeof value === "object" && value !== null && "uri" in value) {
      return true;
    }
  }
  return false;
}

function uploaded(url: string, init: RequestInit): Promise<Response> {
  return new Promise((resolve, reject) => {
    const request = new XMLHttpRequest();
    request.open(init.method ?? "GET", url);
    request.responseType = "text";
    // The multipart of an upload carries the boundary it made, so the content
    // type the page wrote is not copied; every other header is.
    new Headers(init.headers).forEach((value, name) => {
      if (name.toLowerCase() !== "content-type") {
        request.setRequestHeader(name, value);
      }
    });
    request.onload = () => {
      const headers = new Headers();
      for (const line of request.getAllResponseHeaders().trim().split(/\r?\n/)) {
        const split = line.indexOf(":");
        if (split > 0) {
          headers.append(line.slice(0, split).trim(), line.slice(split + 1).trim());
        }
      }
      resolve(new Response(request.responseText, { status: request.status, statusText: request.statusText, headers }));
    };
    request.onerror = () => reject(new TypeError("Network request failed"));
    request.send(init.body as never);
  });
}

// A client of the web keeps the cookie a server sets and sends it back with the
// next request. The fetch of this target keeps none: measured on the login of the
// app of the case, the session POST answered with its cookie and the page that
// followed was fetched as an anonymous visitor, so every screen after a login was
// the one of a guest. The jar is kept here, for the origin of the app only.
const jar = new Map<string, string>();

function readCookies(response: Response): void {
  const header = response.headers.get("set-cookie");
  if (header === null) {
    return;
  }
  for (const piece of header.split(/,(?=[^;=]+=[^;]*)/)) {
    const pair = piece.split(";")[0] ?? "";
    const at = pair.indexOf("=");
    if (at <= 0) {
      continue;
    }
    const name = pair.slice(0, at).trim();
    const value = pair.slice(at + 1).trim();
    if (value === "") {
      jar.delete(name);
    } else {
      jar.set(name, value);
    }
  }
}

function cookieOf(): string | null {
  if (jar.size === 0) {
    return null;
  }
  return Array.from(jar.entries())
    .map(([name, value]) => name + "=" + value)
    .join("; ");
}

globalThis.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
  const url = urlOf(input);
  const same = url.startsWith("/") || url.startsWith(ORIGIN);
  const target = url.startsWith("/") ? ORIGIN + url : url;
  if (init !== undefined && isUpload(init.body)) {
    return uploaded(target, init);
  }
  if (!same) {
    return original(input, init);
  }
  const headers = new Headers(init?.headers);
  const cookie = cookieOf();
  if (cookie !== null && !headers.has("cookie")) {
    headers.set("cookie", cookie);
  }
  const sent = { ...(init ?? {}), headers };
  return original(target, sent).then((response) => {
    readCookies(response);
    return response;
  });
};
`

// A form of the web app submits itself: the browser sends its fields when a
// button of type="submit" inside it is pressed. A native target has no browser,
// and a Pressable carrying type="submit" is a Pressable carrying a prop nothing
// reads -- measured on the login form of fonnso, where pressing the button made
// no request at all. So the form keeps its fields and its button asks it to
// submit.
//
// It is a module of its own and not part of the tag map because the map is not
// the only place a button comes from: an app that paints <button> its own way
// replaces the one of the map, and the form it is inside is still the form.
const nativeForm = `import { createContext, useContext, useMemo, useState, type ReactNode } from "react";

// What a form sends for a file: the part React Native's own uploader, the
// XMLHttpRequest of the target, knows how to read. The fetch of the runtime
// converts parts to bytes and refuses this one, so it never sees it.
export type Upload = { uri: string; name: string; type: string };

export type Form = {
  fields: Map<string, string>;
  // A select that takes more than one value is the only control that answers
  // with more than one value for a name, so it keeps its list apart: the fields
  // of every other control stay one value per name.
  lists: Map<string, string[]>;
  // A file is neither: the form sends the thing itself, and the upload is what
  // fetch of react-native knows how to put in the request.
  files: Map<string, Upload[]>;
  submit: () => void;
};

const FormContext = createContext<Form | null>(null);

// A FormData built out of an element asks the document for the fields of a form.
// There is no document -- the form answered already -- so the constructor hands
// back what it was given. A page that builds its own, to put a file in, gets an
// empty one to append to.
//
// The replacement shares the prototype of the real one, because the fetch of the
// runtime asks whether the body is a FormData before it accepts it: a look-alike
// constructor made every upload "Unsupported BodyInit type" -- measured on the
// device, a form with a file that was read correctly and could not be sent.
const NativeFormData = globalThis.FormData;
const PatchedFormData = function FormData(source?: unknown) {
  if (source instanceof NativeFormData) {
    return source;
  }
  return new NativeFormData();
} as unknown as typeof FormData;
(PatchedFormData as unknown as { prototype: unknown }).prototype = NativeFormData.prototype;
globalThis.FormData = PatchedFormData;

export function FormProvider({ onSubmit, children }: { onSubmit?: unknown; children?: ReactNode }) {
  const [fields] = useState(() => new Map<string, string>());
  const [lists] = useState(() => new Map<string, string[]>());
  const [files] = useState(() => new Map<string, Upload[]>());
  const form = useMemo<Form>(
    () => ({
      fields,
      lists,
      files,
      submit: () => {
        if (typeof onSubmit !== "function") {
          return;
        }
        const body = new NativeFormData();
        for (const [name, value] of fields) {
          body.append(name, value);
        }
        for (const [name, values] of lists) {
          for (const value of values) {
            body.append(name, value);
          }
        }
        for (const [name, uploads] of files) {
          for (const upload of uploads) {
            body.append(name, upload as unknown as Blob);
          }
        }
        (onSubmit as (event: unknown) => void)({ preventDefault() {}, currentTarget: body, target: body });
      },
    }),
    [fields, lists, files, onSubmit],
  );
  return <FormContext.Provider value={form}>{children}</FormContext.Provider>;
}

// The form an element is inside, or nothing when it is not inside one.
export function useForm(): Form | null {
  return useContext(FormContext);
}

// What pressing a button does: whatever the page asked for, and then the form it
// is inside when the button sends it. A browser clicks the button and then sends
// the form, in that order, and a button that does neither is not a button.
//
// It is here and not in the tag map because the map is not the only place a
// button comes from: an app that paints <button> its own way replaces the one of
// the map, and the form it is inside is still the form.
export function usePress(type: unknown, onPress?: () => void): (() => void) | undefined {
  const form = useForm();
  const submit = type === "submit" ? form?.submit : undefined;
  if (onPress === undefined) {
    return submit;
  }
  if (submit === undefined) {
    return onPress;
  }
  return () => {
    onPress();
    submit();
  };
}
`

// A page hands HTML to a target that has no document: `dangerouslySetInnerHTML`
// is the one prop whose whole meaning is a parser, and the renderer drops it
// without a word. Measured on fonnso: the body of every message the assistant
// writes is markdown compiled to HTML, and on the device the bubble came out
// empty -- twenty-four warnings in one screen, and not a paragraph of what the
// assistant said.
//
// So the HTML is read into the tree the tag map already paints: the twenty-two
// tags `markdown()` emits are every one of them in that table, so the only thing
// missing was the reading. A tag the table does not paint keeps its text instead
// of taking the render down with it, and the table says so in development.
//
// It is a module of its own, and asked for only when a page injects HTML, so a
// page that never does pays nothing for it.
const nativeHTML = `export type Node = string | { tag: string; props: Record<string, string>; children: Node[] };

// The elements that close themselves: a parser that waited for their closing tag
// would swallow the rest of the page.
const VOID = ["area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "source", "track", "wbr"];

// The named entities HTML is written with. One that is not here is left as it
// came: a stray & is not worth losing.
const ENTITIES: Record<string, string> = {
  amp: "&",
  lt: "<",
  gt: ">",
  quot: "\"",
  apos: "'",
  nbsp: " ",
  hellip: "\u2026",
  mdash: "\u2014",
  ndash: "\u2013",
};

function decoded(text: string): string {
  return text.replace(/&(#[0-9]+|#[xX][0-9a-fA-F]+|[a-zA-Z][a-zA-Z0-9]*);/g, (whole, name: string) => {
    if (name.charAt(0) !== "#") {
      return ENTITIES[name.toLowerCase()] ?? whole;
    }
    const hexadecimal = name.charAt(1) === "x" || name.charAt(1) === "X";
    const code = parseInt(hexadecimal ? name.slice(2) : name.slice(1), hexadecimal ? 16 : 10);
    if (!Number.isFinite(code) || code <= 0 || code > 0x10ffff) {
      return whole;
    }
    return String.fromCodePoint(code);
  });
}

// The > that closes a tag, and not one inside a quoted value. An attribute
// whose value holds a > is rare and a parser that stopped there would end the
// page at it.
function tagEnd(html: string, from: number): number {
  let quote = "";
  for (let index = from + 1; index < html.length; index += 1) {
    const char = html.charAt(index);
    if (quote !== "") {
      if (char === quote) {
        quote = "";
      }
      continue;
    }
    if (char === '"' || char === "'") {
      quote = char;
      continue;
    }
    if (char === ">") {
      return index;
    }
  }
  return -1;
}

function attributes(source: string): Record<string, string> {
  const props: Record<string, string> = {};
  const pattern = /([^\s=/>]+)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]*)))?/g;
  let found: RegExpExecArray | null;
  while ((found = pattern.exec(source)) !== null) {
    props[found[1].toLowerCase()] = decoded(found[2] ?? found[3] ?? found[4] ?? "");
  }
  return props;
}

// What the page meant, as tags. A closing tag that does not match the one that
// is open is left where it is: HTML that came out of a template or a model is
// not always balanced, and the text under it is worth more than the strictness.
export function parseHTML(html: string): Node[] {
  const root: { tag: string; props: Record<string, string>; children: Node[] } = { tag: "", props: {}, children: [] };
  const open = [root];
  const top = () => open[open.length - 1];
  const text = (value: string) => {
    if (value === "") {
      return;
    }
    // Whitespace between two blocks is nothing on the web, and here it would be
    // an empty run of text between them. Inside a <pre> it is the page.
    if (value.trim() === "" && top().tag !== "pre") {
      return;
    }
    top().children.push(decoded(value));
  };
  let index = 0;
  while (index < html.length) {
    const lt = html.indexOf("<", index);
    if (lt < 0) {
      text(html.slice(index));
      break;
    }
    text(html.slice(index, lt));
    if (html.startsWith("<!--", lt)) {
      const closed = html.indexOf("-->", lt + 4);
      index = closed < 0 ? html.length : closed + 3;
      continue;
    }
    const gt = tagEnd(html, lt);
    if (gt < 0) {
      text(html.slice(lt));
      break;
    }
    const inside = html.slice(lt + 1, gt).trim();
    index = gt + 1;
    if (inside === "" || inside.charAt(0) === "!" || inside.charAt(0) === "?") {
      continue;
    }
    if (inside.charAt(0) === "/") {
      const name = inside.slice(1).trim().toLowerCase();
      for (let at = open.length - 1; at > 0; at -= 1) {
        if (open[at].tag === name) {
          open.length = at;
          break;
        }
      }
      continue;
    }
    const name = /^([a-zA-Z][^\s/>]*)/.exec(inside);
    if (name === null) {
      continue;
    }
    const tag = name[1].toLowerCase();
    const node = { tag, props: attributes(inside.slice(name[1].length)), children: [] as Node[] };
    top().children.push(node);
    const closed = inside.charAt(inside.length - 1) === "/";
    if (!closed && !VOID.includes(tag)) {
      open.push(node);
    }
  }
  return root.children;
}
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
//
// A page watches its own server through a stream, and the browser API for it --
// EventSource -- does not exist on a native target either. The whole of what it
// is lives below, in a module of its own, and ./dom installs it along with the
// rest of what a page expects to find.
const nativeEventSource = `// A stream a page watches is written in the language of a browser: "new
// EventSource(url)". Measured on the app of the case: the whole live path is
// dead here -- "ReferenceError: Property 'EventSource' doesn't exist" -- and the
// chat falls back to asking for the log again, so the answer does not appear
// while it is being written and nothing else arrives as it happens.
//
// The browser API is built on XMLHttpRequest and not on fetch, and not because
// it is shorter: measured on the device, a streaming read through the fetch of
// the app never hands back a body, and the same stream through
// XMLHttpRequest reaches readyState 3 with the first 964 bytes, keeps the
// connection open, and grows by exactly the size of a row each time one is
// written -- 964, 1001, 1038, 1075. That is what a stream is, and it is the only
// road this target has.
//
// A url without a host is the origin of the app, the same way ./net does it for
// fetch: a native client has no origin to resolve against.
import { ORIGIN } from "./origin";

export const CONNECTING = 0;
export const OPEN = 1;
export const CLOSED = 2;

// How long to wait before asking again, doubling to a ceiling. A browser has a
// number of its own and the server can send "retry:"; ours is the one measured
// to reconnect fast enough that a dropped connection does not read as a broken
// one. It resets the moment a stream starts delivering.
const FIRST = 1000;
const MOST = 8000;

export type StreamEvent = { type: string; data?: string; lastEventId?: string };
type Handler = (this: EventSource, event: StreamEvent) => void;

// One chunk of the protocol. An event ends at a blank line, a line that starts
// with a colon carries nothing, and the three fields that matter are "data",
// "id" and "event". The text arrives in pieces that cut anywhere, including the
// middle of a line, so what is not a whole line waits for the next piece.
export class Messages {
  rest: string;
  data: string[];
  id: string | null;
  event: string;

  constructor() {
    this.rest = "";
    this.data = [];
    this.id = null;
    this.event = "";
  }

  push(text: string) {
    this.rest += text;
    const out = [];
    let cut = this.rest.indexOf("\n");
    while (cut >= 0) {
      let line = this.rest.slice(0, cut);
      this.rest = this.rest.slice(cut + 1);
      if (line.endsWith("\r")) {
        line = line.slice(0, -1);
      }
      cut = this.rest.indexOf("\n");
      if (line === "") {
        if (this.data.length > 0) {
          out.push({ data: this.data.join("\n"), id: this.id, event: this.event });
        }
        this.data = [];
        this.event = "";
        continue;
      }
      if (line.charAt(0) === ":") {
        continue;
      }
      const at = line.indexOf(":");
      const field = at < 0 ? line : line.slice(0, at);
      let value = at < 0 ? "" : line.slice(at + 1);
      if (value.charAt(0) === " ") {
        value = value.slice(1);
      }
      if (field === "data") {
        this.data.push(value);
      } else if (field === "id") {
        this.id = value;
      } else if (field === "event") {
        this.event = value;
      }
    }
    return out;
  }
}

function absolute(url: string | URL): string {
  if (typeof url !== "string") {
    return String(url);
  }
  if (url.charAt(0) === "/") {
    return ORIGIN + url;
  }
  return url;
}

export class EventSource {
  url: string;
  withCredentials: boolean;
  readyState: number;
  onopen: Handler | null;
  onmessage: Handler | null;
  onerror: Handler | null;
  lastEventId: string;
  asked: number;
  done: boolean;
  waiting: ReturnType<typeof setTimeout> | null;
  request: XMLHttpRequest | null;
  read: number;
  messages: Messages;
  handlers: Map<string, Handler[]>;

  constructor(url: string | URL, options?: { withCredentials?: boolean }) {
    this.url = absolute(url);
    this.withCredentials = Boolean(options && options.withCredentials);
    this.readyState = CONNECTING;
    this.onopen = null;
    this.onmessage = null;
    this.onerror = null;
    // The id of the last event the server named, which is what it is asked to
    // continue from when the connection has to be made again.
    this.lastEventId = "";
    this.asked = 0;
    this.done = false;
    this.waiting = null;
    this.request = null;
    this.read = 0;
    this.messages = new Messages();
    this.handlers = new Map();
    this.connect();
  }

  addEventListener(name: string, handler: Handler) {
    const list = this.handlers.get(name);
    if (list === undefined) {
      this.handlers.set(name, [handler]);
      return;
    }
    list.push(handler);
  }

  removeEventListener(name: string, handler: Handler) {
    const list = this.handlers.get(name);
    if (list === undefined) {
      return;
    }
    this.handlers.set(name, list.filter((one) => one !== handler));
  }

  dispatch(name: string, event: StreamEvent) {
    const list = this.handlers.get(name);
    if (list !== undefined) {
      for (const handler of list.slice()) {
        handler.call(this, event);
      }
    }
    const own = name === "open" ? this.onopen : name === "message" ? this.onmessage : name === "error" ? this.onerror : null;
    if (typeof own === "function") {
      own.call(this, event);
    }
  }

  // Asking again is what a browser does when a stream ends, and the id travels
  // with the request, so the server continues where it left off instead of
  // sending everything once more.
  again() {
    if (this.done || this.waiting !== null) {
      return;
    }
    const delay = Math.min(FIRST * Math.pow(2, this.asked), MOST);
    this.asked += 1;
    this.waiting = setTimeout(() => {
      this.waiting = null;
      this.connect();
    }, delay);
  }

  connect() {
    if (this.done) {
      return;
    }
    const request = new globalThis.XMLHttpRequest();
    this.request = request;
    this.read = 0;
    this.messages = new Messages();
    request.open("GET", this.url, true);
    request.setRequestHeader("Accept", "text/event-stream");
    if (this.lastEventId !== "") {
      request.setRequestHeader("Last-Event-ID", this.lastEventId);
    }
    request.onreadystatechange = () => {
      this.receive(request);
    };
    request.onerror = () => {
      this.failed();
    };
    request.send();
  }

  receive(request: XMLHttpRequest) {
    if (this.done || request !== this.request) {
      return;
    }
    // A server that refuses -- the thread is not there, the request is not
    // allowed -- refuses the same way however many times it is asked, and asking
    // forever is asking a question whose answer is already known.
    if (request.status >= 400) {
      this.done = true;
      this.readyState = CLOSED;
      this.dispatch("error", { type: "error" });
      return;
    }
    // The headers are in as soon as the connection reaches them, and a server
    // with nothing to say yet sends no text at all: measured, a stream with
    // nothing new stays at the headers and is open all the same. Waiting for
    // text to call it open would leave a page that listens for the opening
    // waiting for something that already happened.
    if (request.readyState >= 2 && this.readyState === CONNECTING) {
      if (request.status === 0) {
        this.failed();
        return;
      }
      this.readyState = OPEN;
      this.asked = 0;
      this.dispatch("open", { type: "open" });
    }
    // Everything the connection has said so far is what has not been read yet:
    // it says the whole of it again and again, each time a little longer.
    if (request.readyState === 3 && request.responseText.length > this.read) {
      const text = request.responseText.slice(this.read);
      this.read = request.responseText.length;
      for (const one of this.messages.push(text)) {
        if (one.id !== null) {
          this.lastEventId = one.id;
        }
        const name = one.event === "" ? "message" : one.event;
        this.dispatch(name, { type: name, data: one.data, lastEventId: this.lastEventId });
      }
      return;
    }
    // The server closed it, or the connection dropped: a stream is not a request
    // that ends, so there is another one to make.
    if (request.readyState === 4) {
      this.failed();
    }
  }

  failed() {
    if (this.done) {
      return;
    }
    this.readyState = CONNECTING;
    this.dispatch("error", { type: "error" });
    this.again();
  }

  close() {
    this.done = true;
    this.readyState = CLOSED;
    if (this.waiting !== null) {
      clearTimeout(this.waiting);
      this.waiting = null;
    }
    if (this.request !== null) {
      this.request.onreadystatechange = null;
      this.request.onerror = null;
      this.request.abort();
      this.request = null;
    }
  }
}
`

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

// A node of this document, in the shape a page asks of one. What it cannot do
// it answers as absent or empty, which is what a page already handles -- and
// every one of these has a measured reason to be here:
//
//   - nodeType is what react-dom checks before it will use a node as the
//     container of a portal, and the body is the container a page asks for by
//     default.
//   - contains and children are how base-ui hides the rest of the app while
//     a dialog is open (markOthers walks the children of the body and marks
//     what the dialog is not): with no contains the walk threw
//     "undefined is not a function" inside FloatingFocusManager every time a
//     sheet was opened, which in a release build is a minified React error.
//   - getRootNode is what tabbable walks up to before it decides what can be
//     focused.
const element = () => ({
  nodeType: 1,
  isConnected: true,
  ownerDocument: null,
  dataset: {},
  children: [],
  childNodes: [],
  firstChild: null,
  lastChild: null,
  parentElement: null,
  parentNode: null,
  nextSibling: null,
  previousSibling: null,
  assignedSlot: null,
  innerHTML: "",
  textContent: "",
  style: { setProperty() {}, removeProperty() {} },
  classList: { add() {}, remove() {}, toggle() {}, contains: () => false },
  setAttribute() {},
  removeAttribute() {},
  hasAttribute: () => false,
  toggleAttribute() {},
  getAttribute: () => null,
  appendChild: (child) => child,
  removeChild() {},
  replaceChildren() {},
  remove() {},
  addEventListener() {},
  removeEventListener() {},
  dispatchEvent: () => true,
  querySelector: () => null,
  querySelectorAll: () => [],
  matches: () => false,
  closest: () => null,
  contains: () => false,
  hasChildNodes: () => false,
  getRootNode() {
    return this;
  },
  // A focus scope keeps guards at the ends of the body by inserting them next to
  // the first and last child, and a node that cannot be inserted leaves the
  // scope with nothing to put there: measured on the first menu the app opened,
  // "document.body.insertAdjacentElement is not a function" stopped its render.
  insertAdjacentElement: () => null,
  remove: () => undefined,
  compareDocumentPosition: () => 0,
  // A node of this target has no box of its own until a view is behind it: the
  // host puts the box a native view measured on the element it hands to a ref,
  // and a popover asks for it to place its content.
  getBoundingClientRect() {
    return this.__rect ?? { top: 0, left: 0, right: 0, bottom: 0, width: 0, height: 0, x: 0, y: 0 };
  },
  getClientRects: () => [],
  focus() {},
  blur() {},
  click() {},
  scrollIntoView() {},
});

// The cookies a page writes through the document, kept for the page itself: one
// assignment is one cookie, and everything after the first semicolon is an
// attribute of it -- expires, path, SameSite -- and not another cookie.
const cookies = new Map();

function cookieText() {
  return Array.from(cookies.entries())
    .map(([name, value]) => name + "=" + value)
    .join("; ");
}

function writeCookie(text) {
  const pair = String(text).split(";")[0] ?? "";
  const at = pair.indexOf("=");
  if (at < 0) {
    return;
  }
  const name = pair.slice(0, at).trim();
  if (name !== "") {
    cookies.set(name, pair.slice(at + 1).trim());
  }
}

const root = element();
root.ownerDocument = root;

// A popup of the web keeps itself inside the viewport by measuring it: the root
// of the document is the box that clips, and a root with no size leaves the
// middleware with nothing to shift against -- measured on the menu of the app of
// the case, it opened at the edge of the screen and ran past it. The window of
// the target is the viewport, and it is told from the host.
const viewport = () => globalThis.__bifrostViewport ?? { width: 400, height: 800 };

Object.defineProperty(root, "clientWidth", {
  get: () => viewport().width,
});
Object.defineProperty(root, "clientHeight", {
  get: () => viewport().height,
});
root.getBoundingClientRect = () => {
  const { width, height } = viewport();
  return { x: 0, y: 0, top: 0, left: 0, right: width, bottom: height, width, height };
};

// The root of the page is where a theme is written: the dark class and the
// color-scheme property. Neither is a class or a property of anything here, so
// both are answered with the only thing the phone understands.
const rootClasses = new Set();
root.classList = {
  add(name) {
    rootClasses.add(name);
    if (name === "dark") {
      applyScheme("dark");
    }
  },
  remove(name) {
    rootClasses.delete(name);
    if (name === "dark") {
      applyScheme("light");
    }
  },
  toggle(name, force) {
    const want = force === undefined ? !rootClasses.has(name) : Boolean(force);
    if (want) {
      root.classList.add(name);
    } else {
      root.classList.remove(name);
    }
    return want;
  },
  contains: (name) => rootClasses.has(name),
};
root.style = {
  setProperty(name, value) {
    if (name === "color-scheme") {
      applyScheme(String(value));
    }
  },
  removeProperty(name) {
    if (name === "color-scheme") {
      applyScheme(null);
    }
  },
  get colorScheme() {
    return currentScheme();
  },
  set colorScheme(value) {
    applyScheme(String(value));
  },
};

const on = (type, listener) => {
  if (!listeners.has(type)) {
    listeners.set(type, new Set());
  }
  listeners.get(type).add(listener);
};

const off = (type, listener) => {
  listeners.get(type)?.delete(listener);
};

// The color scheme is the one thing a phone has and a page says with two other
// names: the "dark" class on the root and the "color-scheme" property. Both are
// answered here, and both end where the phone keeps it -- Appearance -- so a
// dark: class of the stylesheet, which compiles to a prefers-color-scheme
// query, follows what the page asked for. Measured on the user menu of fonnso:
// picking dark moved the marker of the picker and nothing else; the app stayed
// light because the class went to a classList that answered nothing.
const appearance = () => {
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    return require("react-native").Appearance;
  } catch {
    return null;
  }
};

const currentScheme = () => {
  const one = appearance();
  return (one && one.getColorScheme && one.getColorScheme()) || "light";
};

const applyScheme = (want) => {
  const value = want === "dark" || want === "light" ? want : null;
  // The stylesheet of the target answers its dark: classes from the color scheme
  // of its own runtime, and that runtime has a switch for it -- the one of the
  // page of its docs. The phone gets it too, so the controls and the keyboard
  // follow. Measured on the user menu of fonnso: with the phone alone the class
  // dark: of the app never flipped.
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const css = require("react-native-css/native");
    if (css && css.colorScheme && typeof css.colorScheme.set === "function") {
      css.colorScheme.set(value);
    }
  } catch {
    // An app without the style runtime of this target keeps the phone's scheme.
  }
  const one = appearance();
  if (one && typeof one.setColorScheme === "function") {
    one.setColorScheme(value);
  }
};

const query = (media) => {
  const dark = /prefers-color-scheme:\s*dark/.test(String(media));
  const light = /prefers-color-scheme:\s*light/.test(String(media));
  return {
    media: String(media),
    onchange: null,
    get matches() {
      if (dark) {
        return currentScheme() === "dark";
      }
      if (light) {
        return currentScheme() === "light";
      }
      return false;
    },
    addEventListener: on,
    removeEventListener: off,
    addListener: on,
    removeListener: off,
  };
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

// A page asks where it is. There is no browser to answer, so the answer is built
// from the two things that are true: the address of the screen on top, which
// that screen wrote down as it rendered, and the origin the pages are served
// from. The page of the web app at /login is served by the same server a page
// talks to, so the url a page builds out of window.location.origin is the one
// the browser would have built.
const { ORIGIN } = require("./origin");
const { getAddress, setAddress, hasPage, onAddressChange } = require("./route");
const { refresh } = require("./refresh");
const { follow } = require("./intent");

const router = () => require("expo-router").router;
const Linking = () => require("react-native").Linking;

// A url that is not a page of this app and that the app cannot answer: on the
// web the browser leaves the document and paints whatever the server has for it,
// and here the browser is another app.
//
// One url of this app's own server cannot be handed over: this app is what the
// system opens that domain with -- the App Link of fonnso.com is its own, which
// is the whole point of the target -- so asking the system to open it opens this
// app again, with the same url, and the same answer, forever. Measured on a link
// of a mail that had already been used: 1158 launches of the app in one minute,
// and the phone had to be restarted. So a document of this server is left where
// it is and said out loud, and nothing is opened.
//
// What is left is a url of another server -- the dance of an oauth provider, a
// checkout -- and that one the system does open. It is remembered for a moment
// so that the same url is not asked for twice in a row, which is how a loop
// starts when something else in the middle keeps answering the same way.
const ELSEWHERE = new Map();
const ELSEWHERE_WHILE = 10000;

function openElsewhere(target) {
  const url = absolute(String(target));
  if (url.indexOf(ORIGIN) === 0) {
    console.warn(
      "bifrost: " + url + " is a document of this app's own server and no screen of this app can paint it; the app stays where it is, because asking the system to open that domain opens this app again",
    );
    return;
  }
  const at = Date.now();
  for (const [one, when] of ELSEWHERE) {
    if (at - when > ELSEWHERE_WHILE) {
      ELSEWHERE.delete(one);
    }
  }
  if (ELSEWHERE.has(url)) {
    console.warn("bifrost: " + url + " was handed to the browser a moment ago; not again");
    return;
  }
  ELSEWHERE.set(url, at);
  Linking().openURL(url).catch(() => {
    console.warn("bifrost: nothing on this device could open " + url);
  });
}

// The address cut the way a url is cut: path, query and fragment.
const where = () => {
  const at = getAddress();
  const query = at.indexOf("?");
  const hash = at.indexOf("#");
  const ends = [];
  if (query >= 0) ends.push(query);
  if (hash >= 0) ends.push(hash);
  const stop = ends.length === 0 ? at.length : Math.min.apply(null, ends);
  return {
    pathname: at.slice(0, stop) || "/",
    search: query < 0 ? "" : at.slice(query, hash < 0 ? at.length : hash),
    hash: hash < 0 ? "" : at.slice(hash),
  };
};

const scheme = ORIGIN.slice(0, ORIGIN.indexOf("://"));
const authority = ORIGIN.slice(ORIGIN.indexOf("://") + 3);
const port = authority.indexOf(":") < 0 ? "" : authority.slice(authority.indexOf(":") + 1);

const absolute = (to) => {
  if (/^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(to)) {
    return to;
  }
  if (to.charAt(0) === "/") {
    return ORIGIN + to;
  }
  // A relative url is resolved against the directory of the current one, and
  // that is a piece of a browser this shim does not carry. Saying so is better
  // than guessing a page and painting it.
  throw new Error("bifrost: window.location needs a rooted or absolute url, got " + to);
};

// Set while the shim is the one asking the router to move. A change it did not
// ask for is a move of the router -- the back button, a gesture, a link -- and
// that is the one a page hears as popstate, the same as in a browser.
let moving = false;

const go = (to, replace) => {
  const target = absolute(String(to));
  const page = target.indexOf(ORIGIN) === 0 ? target.slice(ORIGIN.length) || "/" : null;
  if (page !== null && hasPage(page)) {
    if (page !== getAddress()) {
      moving = true;
      setAddress(page);
    }
    if (replace) {
      router().replace(page);
    } else {
      router().push(page);
    }
    return;
  }
  // Not a page of this app. On the web the browser leaves the document and the
  // server answers -- a link that sets the session cookie, a route that starts an
  // oauth dance -- and the client that has to make that request is this one, so
  // that the cookie of the answer belongs to the client that will use it.
  follow(target).then((answer) => {
    if (answer !== null) {
      if (answer.kind === "outside") {
        openElsewhere(answer.href);
        return;
      }
      if (answer.href !== getAddress()) {
        moving = true;
        setAddress(answer.href);
      }
      if (replace) {
        router().replace(answer.href);
      } else {
        router().push(answer.href);
      }
      return;
    }
    // The server answered with no move, so what it has is a document -- a
    // preview, the text of a link that expired -- and a document is the
    // browser's. Handing it over is not always possible: see openElsewhere.
    openElsewhere(target);
  });
};

const location = {
  get href() {
    return ORIGIN + getAddress();
  },
  set href(to) {
    go(to, false);
  },
  origin: ORIGIN,
  protocol: scheme + ":",
  host: authority,
  hostname: port === "" ? authority : authority.slice(0, authority.indexOf(":")),
  port,
  get pathname() {
    return where().pathname;
  },
  get search() {
    return where().search;
  },
  get hash() {
    return where().hash;
  },
  assign: (to) => go(to, false),
  replace: (to) => go(to, true),
  reload: () => { void refresh(); },
  toString() {
    return this.href;
  },
};

// The state of the entry on screen, and the entries a page pushed over it with
// no url to move to. A browser keeps those in its history; here they are kept
// here, so that a page which pushes one to catch the back button -- an image
// viewer that closes instead of leaving the page -- is answered the same way.
let state = null;
const pushed = [];

const history = {
  get state() {
    return state;
  },
  pushState(next, _title, to) {
    state = next === undefined ? null : next;
    if (to === undefined || to === null || to === "") {
      pushed.push(true);
      return;
    }
    // A navigation of its own cuts whatever was pushed on top, and a page that
    // pushed nothing loses nothing.
    pushed.length = 0;
    go(to, false);
  },
  replaceState(next, _title, to) {
    state = next === undefined ? null : next;
    if (to === undefined || to === null || to === "") {
      return;
    }
    go(to, true);
  },
  back() {
    if (pushed.length > 0) {
      pushed.pop();
      state = null;
      setTimeout(() => fire("popstate"), 0);
      return;
    }
    router().back();
  },
  forward() {
    router().forward();
  },
  go(step) {
    if (step === 0) {
      void refresh();
      return;
    }
    if (step < 0) {
      history.back();
      return;
    }
    history.forward();
  },
};

// What the document answers when something fires an event at it: every listener
// of that type, the last one first -- the dialog that opened last is the one on
// top -- and whether there was anyone listening at all. Measured on the sidebar
// of fonnso: a dialog of the page is dismissed with Escape through a keydown on
// the document, the phone has no Escape, and the back button took the whole app
// away instead of closing the sheet. The root asks this before it lets the back
// button go, and a dialog that answers keeps it.
const dispatch = (event) => {
  const given = typeof event === "string" ? { type: event } : event;
  const set = listeners.get(given && given.type);
  if (!set || set.size === 0) {
    return false;
  }
  for (const listener of [...set].reverse()) {
    listener(given);
  }
  return true;
};

const fire = (type) => {
  dispatch({ type });
};

onAddressChange(() => {
  if (moving) {
    moving = false;
    return;
  }
  // Deferred, because the address changes while a screen renders and a page
  // that listens for this usually sets state of its own.
  setTimeout(() => fire("popstate"), 0);
});

const install = () => {
  const target = globalThis;
  target.window ??= target;
  // A page asks how wide the window is to decide what to paint -- a menu of a
  // phone instead of the one of a desktop -- and the window of the target is the
  // one the host measured.
  if (target.innerWidth === undefined) {
    Object.defineProperty(target, "innerWidth", { configurable: true, get: () => viewport().width });
  }
  if (target.innerHeight === undefined) {
    Object.defineProperty(target, "innerHeight", { configurable: true, get: () => viewport().height });
  }
  // A control asks what a node is with instanceof -- the slider of a page asks
  // whether its form is an HTMLFormElement -- and a name that does not exist is a
  // ReferenceError and not a false. Measured on the app of the case: the first
  // screen stopped its render with "Property 'HTMLFormElement' doesn't exist".
  // Nothing here is an element of a browser, so every answer is no.
  for (const name of ["Element", "HTMLElement", "HTMLFormElement", "HTMLInputElement", "HTMLButtonElement", "HTMLSelectElement", "HTMLTextAreaElement", "HTMLLabelElement", "HTMLAnchorElement", "HTMLImageElement", "HTMLCanvasElement", "HTMLVideoElement", "HTMLAudioElement", "HTMLDivElement", "HTMLSpanElement", "HTMLParagraphElement", "HTMLTableElement", "HTMLTableRowElement", "HTMLTableCellElement", "HTMLUListElement", "HTMLLIElement", "HTMLHeadingElement", "HTMLStyleElement", "HTMLScriptElement", "HTMLLinkElement", "HTMLMetaElement", "HTMLIFrameElement", "HTMLDialogElement", "HTMLDetailsElement", "SVGElement", "Document", "HTMLDocument", "DocumentFragment"]) {
    target[name] ??= class {};
  }
  // A control keeps a native element in step with its own state by writing
  // through the property descriptor of the prototype -- the select of a language
  // toggle writes its value that way -- and a prototype with no descriptor is a
  // crash: measured, "Cannot read property 'set' of undefined" stopped the first
  // screen that held a select. The setter keeps what it is given on the object
  // it is called on, which is what the control then reads back.
  const property = (name, key) => {
    const ctor = target[name];
    if (ctor === undefined || Object.getOwnPropertyDescriptor(ctor.prototype, key) !== undefined) {
      return;
    }
    Object.defineProperty(ctor.prototype, key, {
      configurable: true,
      get() {
        return this["_" + key];
      },
      set(next) {
        this["_" + key] = next;
      },
    });
  };
  for (const name of ["HTMLInputElement", "HTMLTextAreaElement", "HTMLSelectElement"]) {
    property(name, "value");
  }
  property("HTMLInputElement", "checked");
  // The target installs a location and a history of its own when the app does not
  // ask it not to, and theirs answer the url of the bundler that served the
  // bundle, which is where the page is not: a page that asks where it is would be
  // told it is somewhere else. The generated app.json turns that one off -- the
  // runtime reads extra.router.origin, and false means it installs none -- so the
  // address a page reads is the address of the page. When one is already here anyway, saying
  // so beats answering silently with an address that belongs to no page.
  if (target.location && target.location !== location) {
    console.warn(
      "bifrost: the runtime installed a window.location of its own, so a page that reads it is told where the bundler is instead of where it is; the target wants extra.router.origin set to false",
    );
  }
  target.location ??= location;
  target.history ??= history;
  target.getComputedStyle ??= () => computed;
  target.document ??= {
    documentElement: root,
    body: root,
    head: root,
    defaultView: target,
    title: "",
    // A page walks up from a node to the root of the tree it is in, and the walk
    // ends where the root answers that it is not a shadow root. This document is
    // that root and this is a node that cannot be asked for one.
    getRootNode: () => root,
    // A cookie the page writes -- a theme, a locale, an anonymous id -- is kept
    // and answered back, because the library that writes it reads it again. It
    // cannot reach the requests of the target: those carry the jar of the target,
    // and a cookie the server has to see is one the server sets. Measured on the
    // app of the case: assigning document.cookie threw "Cannot assign to property
    // 'cookie' which has only a getter", and the first effect of its layout
    // stopped there.
    get cookie() {
      return cookieText();
    },
    set cookie(value) {
      writeCookie(value);
    },
    visibilityState: "visible",
    createElement: () => {
      const node = element();
      node.ownerDocument = target.document;
      return node;
    },
    createElementNS: () => {
      const node = element();
      node.ownerDocument = target.document;
      return node;
    },
    createTextNode: () => ({}),
    getElementById: () => null,
    createTreeWalker: () => ({ nextNode: () => null }),
    querySelector: () => null,
    querySelectorAll: () => [],
    addEventListener: on,
    removeEventListener: off,
    dispatchEvent: dispatch,
  };
  target.matchMedia ??= query;
  // A focus scope walks the nodes of the box it guards by observing them, and a
  // name that does not exist is a throw: measured on the first menu the app
  // opened, "Property 'MutationObserver' doesn't exist" stopped its render. The
  // target has no tree to mutate, so there is nothing to report.
  // A focus scope walks the tabbable nodes of its box with a tree walker, and
  // the filter that says what to walk is a name of the browser: measured on the
  // first menu the app opened, "Property 'NodeFilter' doesn't exist" stopped the
  // walk. There are no nodes to walk here, so the filter only has to exist and
  // the walker only has to end.
  target.NodeFilter ??= {
    SHOW_ALL: 0xffffffff,
    SHOW_ELEMENT: 1,
    SHOW_ATTRIBUTE: 2,
    SHOW_TEXT: 4,
    SHOW_CDATA_SECTION: 8,
    SHOW_ENTITY_REFERENCE: 16,
    SHOW_ENTITY: 32,
    SHOW_PROCESSING_INSTRUCTION: 64,
    SHOW_COMMENT: 128,
    SHOW_DOCUMENT: 256,
    SHOW_DOCUMENT_TYPE: 512,
    SHOW_DOCUMENT_FRAGMENT: 1024,
    SHOW_NOTATION: 2048,
  };
  target.MutationObserver ??= class MutationObserver {
    observe() {}
    disconnect() {}
    takeRecords() {
      return [];
    }
  };
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
  // A page asks whether a node is a shadow root with instanceof, and an
  // instanceof whose right side is undefined is a throw, not a false: nothing
  // here is a shadow root, and for that to be the answer the constructor has to
  // exist. Measured: the walk above of the tabbable of a dialog reached this
  // right after asking a node for its root, and the sheet did not open.
  target.ShadowRoot ??= class ShadowRoot {};
  // The same throw waits behind a box of an event: a library that is handed one
  // asks what kind it is with instanceof, and reads the constructor off the
  // window -- "if (event instanceof win.KeyboardEvent)". Measured on the
  // sidebar of fonnso: the X of the sheet answered "right operand of instanceof
  // is not an object" and, in a release build, that same throw ends the app.
  // Nothing here is one of those events, and for that to be the answer the
  // names have to exist.
  target.Event ??= class Event {};
  target.KeyboardEvent ??= class KeyboardEvent {};
  target.FocusEvent ??= class FocusEvent {};
  target.MouseEvent ??= class MouseEvent {};
  target.PointerEvent ??= class PointerEvent {};
  target.addEventListener ??= on;
  target.removeEventListener ??= off;
  target.dispatchEvent ??= () => true;
  target.innerWidth ??= width;
  target.innerHeight ??= height;
  // Where the page is scrolled to. There is one document here and it does not
  // scroll -- the boxes that scroll are boxes of the app -- but a library that
  // places a floating element asks this before it works out a place, and a
  // missing answer is not zero: measured on the user menu of fonnso, floating-ui
  // read pageXOffset of a window that had none, every sum after it came out NaN,
  // and the place it wrote was translate(NaNpx, NaNpx) -- see the rule of
  // ./host about what happens to one of those.
  target.pageXOffset ??= 0;
  target.pageYOffset ??= 0;
  target.scrollX ??= 0;
  target.scrollY ??= 0;
  target.localStorage ??= {
    getItem: (key) => (store.has(key) ? store.get(key) : null),
    setItem: (key, value) => { store.set(key, String(value)); },
    removeItem: (key) => { store.delete(key); },
    clear: () => { store.clear(); },
    key: (index) => [...store.keys()][index] ?? null,
    get length() { return store.size; },
  };
  // A page that watches a stream of its own server writes new EventSource(url),
  // and there is no such thing here. Measured: the live path of the chat of the
  // case was dead, and with it the answer did not appear while it was being
  // written. It is built on XMLHttpRequest and not on fetch, because a streaming
  // read through the fetch of this target never hands back a body and the same
  // stream through XMLHttpRequest arrives in parts -- measured, one connection
  // that grew 964, 1001, 1038, 1075 as rows were written.
  if (target.EventSource === undefined) {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    target.EventSource = require("./events").EventSource;
  }
  if (target.navigator && !target.navigator.clipboard) {
    const api = clipboard();
    target.navigator.clipboard = {
      writeText: async (text) => { await api?.setStringAsync(String(text)); },
      readText: async () => (await api?.getStringAsync()) ?? "",
    };
  }
};

// A portal is a trick of the document: it takes a node out of where it was
// written and puts it somewhere else, to escape a box that clips it. There is no
// document here, and the container a page asks for -- the body, usually -- is the
// object above, which is not a node: react-dom refuses it, and a component that
// only paints its content through a portal throws instead of painting. Measured
// on the case: the tab bar menu of the app, which is a dropdown with a portal
// under it, answered "Target container is not a DOM element", and in a release
// build that reaches the screen as a minified React error with no way to know
// what it was.
//
// A native view paints its children inside itself, over the ones that came
// before, and the tree is the order of the page -- so what a portal was for is
// already true where the portal was written: the children render inline, and the
// overlay is the last thing painted. It is a smaller answer than the browser's
// and it is written down as one: a box that clips its children clips these too.
//
// The patch goes on react-dom itself and not on a copy of it, because @base-ui
// imports the module whole, as a namespace; what it calls is the member of this
// object, whenever it reads it.
const reactDOM = () => {
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    return require("react-dom");
  } catch {
    return null;
  }
};

const portalless = () => {
  const dom = reactDOM();
  if (!dom || typeof dom.createPortal !== "function") {
    return;
  }
  try {
    // A portal is a place of the document: the body of a page, or a container the
    // page named. A fragment is where a component collects nodes it does not want
    // painted -- the items of a select that is closed, the text of a label off to
    // the side -- and a browser keeps them out of the page. Painting them here put
    // the items of a closed select over the first screen: measured on the language
    // toggle of the app of the case.
    dom.createPortal = (children, container) => {
      const document = globalThis.document;
      if (container === undefined || container === null || container === document || container === document?.body || container === document?.documentElement) {
        return children;
      }
      return null;
    };
  } catch {
    console.warn("bifrost: react-dom does not let its createPortal be replaced, so a portal of the page will throw");
  }
};

portalless();
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
	imports.WriteString("import \"../bifrost/net\";\n")
	imports.WriteString("import * as declaredFonts from \"../bifrost/fonts\";\n")
	imports.WriteString("import { useFonts } from \"expo-font\";\n\n")
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
import { useEffect } from "react";
import { BackHandler, StyleSheet, View } from "react-native";
import { SafeAreaProvider, useSafeAreaInsets } from "react-native-safe-area-context";

// The back button of the phone, as the Escape of a browser: a dialog of the page
// is dismissed with an Escape keydown that the library listens for on the
// document, and a dialog that answers keeps the back button from leaving the app.
// Measured on the sidebar of fonnso: the sheet was open and the back button took
// the whole app away. With this, the sheet closes and the app stays.
function Back() {
  useEffect(() => {
    const subscription = BackHandler.addEventListener("hardwareBackPress", () => {
      const doc = (globalThis as { document?: { dispatchEvent?: (event: unknown) => boolean } }).document;
      return doc?.dispatchEvent?.({
        type: "keydown",
        key: "Escape",
        preventDefault() {},
        stopPropagation() {},
      }) === true;
    });
    return () => subscription.remove();
  }, []);
  return null;
}

// The keyboard takes the bottom of the screen, and a window that shrinks for it
// is the window of the system: padding the shell by the height of the keyboard
// as well took the room twice. Measured on the login of picsel: with the keyboard
// open the form of the page closed to nothing and the fields went out of sight.
// The window is left to the system, and a page that does not fit scrolls.
function Shell() {
  const insets = useSafeAreaInsets();
  return (
    <View style={[styles.shell, { paddingTop: insets.top, paddingBottom: insets.bottom }]}>
      <Back />
      <Stack screenOptions={{ headerShown: false }} />
    </View>
  );
}

// The typeface of the app is a file a screen has to load, and a native screen
// has no font to fall back to but the one of the system, which is another
// typeface with other widths. Measured on the app of the case, before this
// existed: with the system font the same paragraph of the landing broke at
// another word. So nothing is painted until the fonts of the app are in, and an
// app that ships none is in on the first frame.
const FONTS = declaredFonts.fonts ?? {};
const NONE = Object.keys(FONTS).length === 0;

export default function Layout() {
  const [ready] = useFonts(FONTS);
  if (!NONE && !ready) {
    return null;
  }
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

const nativeIntent = `import { ORIGIN } from "./origin";

// The path and the query of a url, cut the way the router and the server talk
// about them. A deep link arrives whole -- a scheme, a host, and from a
// development bundler a '/--/' in front of it -- while a redirect answers with
// a path, or with a url of its own.
export function pathOf(url: string): string {
  const bundler = url.indexOf("/--/");
  if (bundler >= 0) {
    return url.slice(bundler + 3) || "/";
  }
  const scheme = url.indexOf("://");
  // Without a scheme there is no host to cut off, and what is left is a path of
  // this app whether or not it was written with the slash in front.
  if (scheme < 0) {
    return url.charAt(0) === "/" ? url : "/" + url;
  }
  const slash = url.indexOf("/", scheme + 3);
  return slash < 0 ? "/" : url.slice(slash) || "/";
}

// An address as a path of this app's own server, or null when it is somewhere
// else. A url that starts with two slashes is one of the ways an
// address that looks relative leaves this origin, so it is not one of ours.
export function ownPath(address: string): string | null {
  if (address.charAt(0) === "/") {
    return address.charAt(1) === "/" ? null : address;
  }
  if (address.indexOf(ORIGIN) !== 0) {
    return null;
  }
  return address.slice(ORIGIN.length) || "/";
}

// A url of this app's own server that the router has no screen for. In a browser
// the document goes away and the server answers -- a 303, a session cookie, and
// the address to go on from -- and this is the same request made by the app, so
// that the cookie lands where the pages will send it and the answer is one the
// router can paint.
//
// The redirect is read instead of followed, because the address it names is the
// whole answer: a redirect inside this origin means the app stays (the link of a
// mail that sets the session), and one that leaves it means the browser goes (the
// endpoint that starts a checkout). Measured on the app of the case: the fetch of
// the runtime answers a 303 as a 303 when it is told manual, with the
// location saying where it points, while a request that answers 200 is a
// document of the server -- a preview, say -- that no screen of this app has.
export type Answered =
  // The server named another address of this app: the link of a mail that sets
  // the session, a route that moved.
  | { kind: "inside"; href: string }
  // The server named an address that is not of this app: the dance of an oauth
  // provider, the url of a checkout. The browser is the one that goes there, and
  // the app only has to hand it over.
  | { kind: "outside"; href: string };

// What the server answers a url of its own with, when that answer is a move.
//
// It is read instead of followed, because the address it names is the whole
// answer: one inside this origin means the app stays, and one outside means the
// browser goes. It is asked of every url of the server and not left to the
// browser, because the cookie of an answer has to land in the jar the app will
// send with the pages -- measured on the dance of Google: the cookie of the
// state belongs to whoever asked for the provider, and the callback is only
// allowed to follow it from there.
//
// A url the server answers without a move is a document of the server, and a
// document is the browser's: there is nothing to hand back here.
export async function follow(address: string): Promise<Answered | null> {
  const path = ownPath(address);
  if (path === null) {
    return null;
  }
  let response: Response;
  try {
    response = await fetch(ORIGIN + path, { redirect: "manual" });
  } catch {
    return null;
  }
  if (response.status < 300 || response.status > 399) {
    return null;
  }
  const at = response.headers.get("location");
  if (at === null || at === "") {
    return null;
  }
  const ours = ownPath(at);
  return ours === null ? { kind: "outside", href: at } : { kind: "inside", href: ours };
}
`

// The intent of the system, which is what turns the url a link opened the app
// with into a move of the router. A path that is a page is the router's, exactly
// as it was before this file existed. A path that is not is a url of the server,
// and the server is who the link is talking to: the app asks for it the way a
// browser would and moves to the address the answer names, and what the server
// does not answer with a redirect is left alone, which is where it landed before.
const nativeIntentFile = `import { Linking } from "react-native";
import { follow, pathOf } from "../bifrost/intent";
import { hasPage } from "../bifrost/route";

// The intent of the system is the url a link opened the app with. A path that is
// a page is the router's, exactly as it was before this file existed, and a path
// that is not is a url of the server: the server is who the link is talking to,
// so it is asked the way a browser would ask it and the app moves to the address
// the answer names.
//
// A redirect that leaves this server is the one case where the browser has to
// go: the dance of an oauth provider happens there and comes back to this app as
// another url of this domain.
//
// And a url of this server that the server answers with no move is a document,
// and this app cannot hand it over: the App Link of this domain is its own, so
// asking the system to open it opens this app again with the same url, which is
// how a link of a mail that had already been used blocked a phone -- measured,
// 1158 launches in one minute. It is said and nothing is opened.
export async function redirectSystemPath({ path }: { path: string; initial: boolean }) {
  const address = pathOf(path);
  if (hasPage(address)) {
    return path;
  }
  const answer = await follow(address);
  if (answer !== null) {
    if (answer.kind === "outside") {
      await Linking.openURL(answer.href).catch(() => {
        console.warn("bifrost: nothing on this device could open " + answer.href);
      });
      return null;
    }
    return answer.href;
  }
  console.warn(
    "bifrost: " + address + " is a document of this app's own server and no screen of this app can paint it; the app stays where it is, because asking the system to open that domain opens this app again",
  );
  return null;
}
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
  // The accept asks for the props and also takes anything, because a server that
  // reads the header as one string -- a context of the page that only loads for
  // a document, a middleware of an app -- has to see that the request is one it
  // would have answered with a page. Measured on picsel: its context, and with it
  // the user of the session, loaded only for a text/html or */* accept, so every
  // page after the login was the page of a guest.
  const response = await fetch(ORIGIN + pathname, { headers: { Accept: DATA_MEDIA_TYPE + ", */*" } });
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
import { ORIGIN } from "./origin";
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

// A link of a page goes through the same door window.location goes through, and
// not straight to the router. The two are not the same thing: a url of this
// server that is not a screen of this app is not a route -- a link that sets the
// session cookie, the url that starts an oauth dance, a document like a preview
// -- and the router is the one thing that cannot answer it. Measured on the
// button of Google of the app: the router was handed /api/auth/google and
// painted "Unmatched Route", which is what it says when a url is not one of its
// own. What knows the difference is the shim of window.location, so the link
// asks it.
export async function navigate(href: string): Promise<void> {
  if (href.charAt(0) === "/" || href.indexOf(ORIGIN) === 0) {
    window.location.href = href;
    return;
  }
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
    forward: () => { console.warn("bifrost: a screen has no forward history to go to; the app stays where it is"); },
  };
}
`

const nativeScreen = `import { router, useIsFocused, useLocalSearchParams, usePathname } from "expo-router";
import { Component, useEffect, useRef, useState, type ReactNode } from "react";
import { ActivityIndicator, Animated, StyleSheet, Text, type View as NativeView } from "react-native";
import { StyledView as View } from "./text";
import { DocumentScroll } from "./document-scroll";
import { loadPage, type PageData } from "./data";
import { onRefresh } from "./refresh";
import { setAddress } from "./route";

type State =
  | { status: "loading" }
  | { status: "ready"; data: PageData }
  | { status: "failed"; message: string };

// The props to render with when the page did not survive its own render: the
// error.tsx the generator put at this level, and what it should be told.
type Fallback = { level: number; message: string };

export type ScreenProps = {
  render: (props: Record<string, unknown>, pageKey?: string) => ReactNode;
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
  const [viewportHeight, setViewportHeight] = useState(0);
  const [contentHeight, setContentHeight] = useState(0);
  const scrollY = useRef(new Animated.Value(0)).current;
  const innerView = useRef<NativeView | null>(null);
  const href = pathname + query(search, params);

  // Where this screen is, written down before anything inside it renders: a page
  // reads the address in its first effect, and the effects of a child run before
  // the effect of its parent. Only the screen on top writes it -- a stack keeps
  // the screens behind this one mounted, and they are not where the app is.
  if (focused) {
    setAddress(href);
  }

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
  //
  // The root of a browser is here too, and not as decoration: the stylesheet of
  // the app still writes the typeface, the line height and the colours of html
  // and body, and the selector rewrite names those two tags with a class. A
  // screen is the document of this page, so it paints them. Measured: with the
  // preflight of Tailwind expanded, the default typeface of the theme is a rule
  // of html, and a screen without this painted every text with the font of the
  // system.
  const direction = document.dir === "rtl" || document.dir === "ltr" ? document.dir : undefined;
  const root = ["native-html", "native-body", document.class].filter((one) => typeof one === "string" && one !== "").join(" ");
  // The document of a browser scrolls, and a screen of this target is its own
  // document: measured on the device, a page taller than the screen without a
  // class that scrolls painted the first rows and nothing else, with no way to
  // reach the rest. The box that scrolls is the screen itself and never a class
  // of the page, like the document on the web, and a page that names its own
  // scroll box, by h-screen with overflow, is a box that fits and leaves this one
  // with nothing to scroll.
  const documentStyle = direction === undefined ? styles.screen : [styles.screen, { direction }];
  return (
    <View className={root} style={documentStyle}>
      <DocumentScroll.Provider value={{ y: scrollY, viewport: viewportHeight, content: contentHeight, inner: innerView }}>
        <Animated.ScrollView
          contentContainerStyle={styles.page}
          innerViewRef={innerView as never}
          keyboardShouldPersistTaps="handled"
          onContentSizeChange={(width: number, height: number) => { setContentHeight(height); }}
          onLayout={(event) => { setViewportHeight(event.nativeEvent.layout.height); }}
          onScroll={Animated.event([{ nativeEvent: { contentOffset: { y: scrollY } } }], { useNativeDriver: true })}
          scrollEventThrottle={16}
          style={styles.pageScroll}
        >
          {content}
        </Animated.ScrollView>
      </DocumentScroll.Provider>
    </View>
  );
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
  screen: { flex: 1 },
  pageScroll: { flex: 1 },
  page: { flexGrow: 1 },
  center: { flex: 1, alignItems: "center", justifyContent: "center", padding: 24 },
  error: { color: "crimson", fontSize: 12, textAlign: "center" },
});
`

const nativeDocumentScroll = `import { createContext, useContext, type RefObject } from "react";
import type { Animated, View } from "react-native";

// The offset of the document, measured by the screen that scrolls it. A box the
// page pinned to the viewport is inside the content, so the content scrolls under
// it and the box has to move by as much as the content moved. A screen that does
// not hold the document -- a modal, a test -- hands nothing, and the box stays
// where its layout put it.
export type DocumentScrollValue = {
  y: Animated.Value;
  viewport: number;
  content: number;
  inner: RefObject<View | null> | null;
};

export const DocumentScroll = createContext<DocumentScrollValue | null>(null);

export function useDocumentScroll(): DocumentScrollValue | null {
  return useContext(DocumentScroll);
}
`

// A block that scrolls is written in the stylesheet, and a native view only
// scrolls when it is a ScrollView: `overflow-y-auto` is not a declaration native
// can express, so the compiler drops it -- with a warning for `overflow-auto` and
// with none at all for `overflow-x-auto` and `overflow-y-auto` -- and the block
// grows past the screen and is clipped. Measured on the app of the case: 36 uses
// in 16 files, and every panel of the chat (files, vault, slack, the invitations)
// came out with its content cut and no way to reach the rest.
//
// A page names the scroll with a class, so the class is what the map reads. The
// decision lives in a module of its own, in plain JavaScript, because the
// transformer reads it too: a class the map paints is not a class the target
// dropped, and saying it was would be a lie in the log.
const nativeScroll = `// The classes that ask a block to scroll, by the direction they ask for. What a
// browser calls a scroll is a box with the content clipped inside it; here it is
// a ScrollView, and the direction decides which way it scrolls.
const VERTICAL = ["overflow-y-auto", "overflow-y-scroll", "overflow-auto", "overflow-scroll"];
const HORIZONTAL = ["overflow-x-auto", "overflow-x-scroll"];

// What a class asks for, or nothing when it asks for no scroll at all. The one
// that hides is a clip and not a scroll, and it is left to the compiler, which
// expresses it as it is.
function scrollOf(className) {
  if (typeof className !== "string" || className === "") {
    return null;
  }
  const names = className.split(/\s+/);
  const down = names.some((name) => VERTICAL.indexOf(name) >= 0);
  const across = names.some((name) => HORIZONTAL.indexOf(name) >= 0);
  if (down && across) {
    // A native ScrollView scrolls one way. A block that asks for both is asking
    // for a box inside a box, and the direction that is worth more -- the one
    // that reaches the content below the fold -- is the one it gets.
    return "vertical";
  }
  if (down) {
    return "vertical";
  }
  if (across) {
    return "horizontal";
  }
  return null;
}

// The layout of the children of a box, which a native ScrollView does not let
// the box hold: React Native refuses a style with alignItems or justifyContent on
// the scroll view itself and does not render at all -- it says "ScrollView child
// layout must be applied through the contentContainerStyle prop".
//
// It is right about the meaning, and the meaning is the browser's: the items of a
// box that scrolls sit along the middle of the box. So the two families of class
// that say it come out of the box and go to the content, and a class of a variant
// -- md:items-center -- is the same class for this.
const IN_CONTENT = /^!?(items|justify)-(.+)$/;

const LAYOUT = {
  items: {
    start: "flex-start",
    end: "flex-end",
    center: "center",
    baseline: "baseline",
    stretch: "stretch",
  },
  justify: {
    start: "flex-start",
    end: "flex-end",
    center: "center",
    between: "space-between",
    around: "space-around",
    evenly: "space-evenly",
  },
};

// What is left for the box, and what the content has to carry instead. The
// class-derived style is written by the compiler from the class itself, and the
// map cannot take a declaration out of it: the class is the only handle there is,
// so the class is what moves.
function splitScroll(className) {
  const names = String(className === undefined || className === null ? "" : className)
    .split(/\s+/)
    .filter((name) => name !== "");
  const box = [];
  const content = {};
  for (const name of names) {
    const at = name.lastIndexOf(":");
    const last = at < 0 ? name : name.slice(at + 1);
    const found = IN_CONTENT.exec(last);
    if (found === null) {
      box.push(name);
      continue;
    }
    const value = LAYOUT[found[1]][found[2]];
    if (value === undefined) {
      // A value this does not know: the box cannot hold it either way, because
      // the renderer refuses the declaration, so it is dropped and said.
      if (typeof console !== "undefined") {
        console.warn("bifrost: " + name + " has no counterpart on a box that scrolls");
      }
      continue;
    }
    content[found[1] === "items" ? "alignItems" : "justifyContent"] = value;
  }
  return { className: box.join(" "), content };
}

// A child a scroll view can pin: position: sticky is no style a native view has,
// and the scroll view is what answers it, by holding the child it was asked to
// stick. The class is the only handle there is, because the compiler drops the
// position and the view never reads it.
function isSticky(className) {
  return typeof className === "string" && className.split(/\s+/).indexOf("sticky") >= 0;
}

// A child the page pins to the viewport: position: fixed is no style a native
// view has, and ./position turns it into absolute before the stylesheet is
// compiled -- an absolute box is still a box inside the content, so this is what
// names the one that has to be moved by as much as the content moved.
function isFixed(className) {
  return typeof className === "string" && className.split(/\s+/).indexOf("fixed") >= 0;
}

// A fixed box the page anchors to the bottom of the viewport: the bottom of an
// absolute box is the one of the box that holds it, and for a page that scrolls
// that box is the whole content, so the difference between the document and the
// screen is what has to be taken out.
function isBottomAnchored(className) {
  return typeof className === "string" && className.split(/\s+/).some((name) => name.indexOf("bottom-") === 0);
}

module.exports = { scrollOf, splitScroll, isSticky, isFixed, isBottomAnchored, VERTICAL, HORIZONTAL };
`

const nativeRuns = `// A block that holds a string has to become one Text, because only a Text may
// hold a string -- and which strings are one text and which are boxes of their
// own is not a detail of the target: a browser decides it the same way, and it
// decides it from what lays the children out.
//
// In a block container the children flow as text, so two spans side by side are
// one line, and wrapping them together is what a paragraph with a bold word in
// the middle needs. In a flex container every child is a box of its own: the
// same two spans are two flex items, and the browser puts one at each end of the
// line when the container asks for justify-between.
//
// So a container that asks for flex does not absorb an element into the run.
// Measured on fonnso: the footer of the landing page is a flex row with
// justify-between around two spans, and it came out as the single line
// "Fonnso3 Lines Studio" -- the two items merged, with nowhere for the space
// between them to go.
const FLEX = ["flex", "inline-flex"];

// Whether the class list asks for a flex container. The names are compared whole
// because flex-1 is not a display: it is how much of the box an item takes, and
// it says nothing about who lays out the children.
function asksForFlex(className) {
  if (typeof className !== "string" || className === "") {
    return false;
  }
  return className.split(/\s+/).some((name) => FLEX.indexOf(name) >= 0);
}

// The children of a block, with the fragments taken out of the way. A fragment
// is nothing to a browser: its children are the children of the box that holds
// it, so a run of text crosses it. Measured on the organization panel of the
// case: a slug, a slash and a button, with the slash written inside a fragment
// -- the string stayed out of the run, which is out of the only element that can
// hold one: react-native says so ("Text strings must be rendered within a
// <Text> component"), the slash is not painted, and the button behind it falls
// to the next line where the browser has both on the same one.
function flatten(children, isFragment) {
  if (typeof isFragment !== "function") {
    return children;
  }
  const out = [];
  for (const child of children) {
    if (Array.isArray(child)) {
      out.push(...flatten(child, isFragment));
      continue;
    }
    if (isFragment(child)) {
      const inner = child.props === null || child.props === undefined ? [] : child.props.children;
      out.push(...flatten(Array.isArray(inner) ? inner : [inner], isFragment));
      continue;
    }
    out.push(child);
  }
  return out;
}

// The children of a block, with each run of text wrapped into one element and
// nothing else touched. isInline says whether a child is one of the elements the
// target paints as a run of text -- a span and its family -- and the caller is
// who knows, because the map of an app is not the map of the generator.
// wrapText is the element a run of text goes into, and it is a callback because
// this file has no JSX. isFragment says which child is a fragment, and the
// caller knows that for the same reason.
function runsOf(children, className, isInline, wrapText, isFragment) {
  const runs = [];
  let text = [];
  const close = () => {
    if (text.length > 0) {
      const run = text;
      text = [];
      runs.push(wrapText(run, runs.length));
    }
  };
  const flex = asksForFlex(className);
  for (const child of flatten(children, isFragment)) {
    const bare = typeof child === "string" || typeof child === "number";
    // A run of text is one item of a flex container and an element is another,
    // so in a flex container the run is closed instead of letting the element
    // join it.
    if (!bare && (!isInline(child) || flex)) {
      close();
      runs.push(child);
      continue;
    }
    text.push(child);
  }
  close();
  return runs;
}

module.exports = { runsOf, asksForFlex };
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

const nativePlace = `// A page that places a popup works out where it goes by measuring the page, and
// what it measured travels as a style of a browser: a box fixed to the viewport
// and a transform of translate(544px, 130px). react-native has neither -- an
// absolute box stands for the fixed one, and a list of transforms stands for the
// translate -- and a translate whose numbers are not numbers is no place at all.
// The same mapping answers a box of the app that places itself, and the style a
// list of styles holds.
export function placed(style: unknown): unknown {
  if (Array.isArray(style)) {
    return style.map(placed);
  }
  if (style === null || typeof style !== "object") {
    return style;
  }
  const given = { ...(style as Record<string, unknown>) };
  if (given.position === "fixed") {
    // A popup of the web is fixed to the viewport and places itself with a
    // transform. The target has a layer for those boxes and the box that brings
    // its own place is laid out inside it: where a browser had a viewport, this
    // has the layer of the portal.
    if (given.transform !== undefined) {
      delete given.transform;
      delete given.top;
      delete given.bottom;
      delete given.maxHeight;
      given.position = "absolute";
      given.left = 0;
      given.right = 0;
      given.alignItems = "center";
    } else {
      given.position = "absolute";
    }
  }
  for (const name of Object.keys(given)) {
    if (name.startsWith("--") || (name === "minWidth" && given[name] === "max-content")) {
      delete given[name];
    }
  }
  // A popup of the web hides itself while it is being measured, and a target
  // that answered the measurement with nothing would keep it out of sight for
  // good: the key is dropped and the box paints where it was placed.
  if (given.visibility !== undefined) {
    delete given.visibility;
  }
  const transform = given.transform;
  if (typeof transform === "string") {
    const translate = /^translate\(\s*(-?[\d.]+)px\s*,\s*(-?[\d.]+)px\s*\)$/.exec(transform.trim());
    if (translate === null) {
      delete given.transform;
    } else {
      given.transform = [{ translateX: Number(translate[1]) }, { translateY: Number(translate[2]) }];
    }
  }
  return given;
}
`

const nativeHostHead = `import { Children, Fragment, cloneElement, createElement, isValidElement, useRef, useState, useSyncExternalStore, type ReactNode } from "react";
import { Dimensions, Image, Linking, Modal, Platform, ScrollView, StyleSheet } from "react-native";
import DateTimePicker from "@react-native-community/datetimepicker";
import Slider from "@react-native-community/slider";
import * as DocumentPicker from "expo-document-picker";
import Svg, { Circle, ClipPath, Defs, Ellipse, G, Line, LinearGradient, Mask, Path, Polygon, Polyline, RadialGradient, Rect, Stop, SvgUri, Symbol as SymbolTag, Use } from "react-native-svg";
import { styled } from "react-native-css/native";
import { StyledAnimatedView as AnimatedView, StyledPressable as Pressable, StyledStickyView as StickyView, StyledText as Text, StyledTextInput as TextInput, StyledView as View, useInheritedColor } from "./text";
import { ORIGIN } from "./origin";
import { isBottomAnchored, isFixed, isSticky, scrollOf, splitScroll } from "./scroll";
import { runsOf, asksForFlex } from "./runs";
import { FormProvider, useForm, usePress, type Upload } from "./form";
import { placed } from "./place";

// A link asks the router when it is pressed, and not when this file is read: the
// map of tags is loaded from the JSX runtime the first time a page paints a
// string tag, and that can be while expo-router is still evaluating. Measured on
// the app of the case: the import was a cycle, expo-router came back half built,
// the map stopped halfway, and the first screen painted an Image that was
// undefined.
function navigate(href: string): void {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const router = require("virtual:bifrost/navigation") as { navigate: (href: string) => unknown };
  void router.navigate(href);
}

// The scroll view of a box that scrolls fills the box, which is what its content
// did in a browser: the box has the size the page gave it and clips what goes
// past it.
const SCROLL_FILLS = { flex: 1 };


// A popup of the web keeps itself inside the viewport, and the viewport of the
// target is the window of the device: the document shim asks for it here.
const viewport = Dimensions.get("window");
(globalThis as { __bifrostViewport?: { width: number; height: number } }).__bifrostViewport = { width: viewport.width, height: viewport.height };
Dimensions.addEventListener("change", ({ window }) => {
  (globalThis as { __bifrostViewport?: { width: number; height: number } }).__bifrostViewport = { width: window.width, height: window.height };
});

// A component of the web asks a React ref for a node of the document: a trigger
// asks its closest form, a popover measures the trigger, focus guards walk the
// tree. A native view is not a node, so the ref hands back the element of the
// document shim, with the native view kept on it. The element and the callback
// are stable for the life of the component: a ref that is a new function on
// every render is detached and attached again, and the library that owns it
// answers with a state of its own -- measured, the language select of the app
// went into "Maximum update depth exceeded".
function useElementRef(ref: unknown) {
  const latest = useRef(ref);
  latest.current = ref;
  const element = useRef<Record<string, unknown> | null>(null);
  const callback = useRef((node: unknown) => {
    const current = latest.current;
    if (node === null || node === undefined) {
      element.current = null;
      if (typeof current === "function") {
        (current as (value: unknown) => void)(null);
        return;
      }
      if (current !== null && typeof current === "object") {
        (current as { current: unknown }).current = null;
      }
      return;
    }
    if (element.current === null) {
      const document = (globalThis as { document?: { createElement?: (tag: string) => Record<string, unknown> } }).document;
      element.current = document === undefined || document.createElement === undefined ? {} : document.createElement("div");
    }
    (element.current as { native?: unknown }).native = node;
    const measured = element.current;
    const measure = (node as { measureInWindow?: (callback: (x: number, y: number, width: number, height: number) => void) => void }).measureInWindow;
    if (typeof measure === "function") {
      measure.call(node, (x: number, y: number, width: number, height: number) => {
        (measured as { __rect?: unknown }).__rect = { x, y, width, height, top: y, left: x, right: x + width, bottom: y + height };
      });
    }
    if (typeof current === "function") {
      (current as (value: unknown) => void)(element.current);
      return;
    }
    if (current !== null && typeof current === "object") {
      (current as { current: unknown }).current = element.current;
    }
  }).current;
  return callback;
}
`

const nativeHostBody = `
// Blocks. They hold other blocks, which a View can.
const BOXES = ["address", "article", "aside", "blockquote", "caption", "dd", "details", "dialog", "div", "dl", "dt", "fieldset", "figcaption", "figure", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "header", "legend", "li", "main", "nav", "ol", "output", "section", "summary", "table", "tbody", "td", "tfoot", "th", "thead", "tr", "ul"];

// Everything whose content is text.
const TEXTS = ["abbr", "b", "br", "cite", "code", "del", "em", "i", "label", "mark", "p", "pre", "q", "s", "small", "span", "strong", "time", "u"];

// Sheet links, metadata and scripts have nothing to paint.
const HIDDEN = ["base", "desc", "head", "link", "meta", "noscript", "script", "style", "title"];

// Props whose whole meaning is the document: a page writes them for the browser,
// a native view has nothing to do with them, and the renderer drops them without
// a word. One case earns a warning: a surface the target does not have --
// contentEditable is dead text here, and nothing says the editing went away. It
// is invisible until somebody compares the two targets, so the runtime names it
// in development. A prop the web gets for free and native does not -- srcset,
// loading -- is not worth a line, because leaving it out changes nothing.
// Measured: htmlFor is not either. It is an association a reader still reads
// through, and fonnso alone writes it twenty-nine times, which would be
// twenty-nine warnings for a paragraph still on the screen.
//
// dangerouslySetInnerHTML was on this list too, and it is not any more: it is
// the body of what a page says, and dropping it left the bubble of every message
// of the assistant empty. It is painted now, and nativeProps is where.
const DOM_ONLY = ["contentEditable"];

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

// The block that scrolls, when the classes ask for it, and a plain View when
// they do not. Two things of a ScrollView are not decoration and both were a
// render error before they were here.
//
// Keeping the keyboard up while a tap lands: with the keyboard up a scroll view
// eats the first tap -- measured on the app of the case, the send button of a
// form focused with the keyboard open had to be pressed twice -- and "handled"
// is what gives the tap to the button that answers it.
//
// And the layout of the children: the two families of class that say where the
// items of a box sit -- items-center, justify-between -- do not belong to the box
// that scrolls, they belong to what it holds. ./scroll takes them out of the
// class and hands them over, which is the only handle there is: the style of a
// box is written by the compiler from its class, and no prop can remove a
// declaration from it.
// The block that scrolls, when the classes ask for it, and a plain View when
// they do not. Three things of a ScrollView are not decoration and all three were
// a render error before they were here.
//
// Keeping the keyboard up while a tap lands: with the keyboard up a scroll view
// eats the first tap -- measured on the app of the case, the send button of a
// form focused with the keyboard open had to be pressed twice -- and "handled"
// is what gives the tap to the button that answers it.
//
// The layout of the children: the two families of class that say where the items
// of a box sit -- items-center, justify-between -- do not belong to the box that
// scrolls, they belong to what it holds. ./scroll takes them out of the class and
// hands them over, which is the only handle there is: the style of a box is
// written by the compiler from its class, and no prop can remove a declaration
// from it.
//
// And the measured box stays the box it was: the ScrollView goes inside a View
// that keeps the class minus what moved. Measured on the app of the case, a box
// that scrolls with the app's own onLayout on the ScrollView put the whole screen
// into a remount cycle -- ~25 warnings a second from react-native-css against 5
// in 30 s without it -- because the height a ScrollView reports is not the height
// the parent measured from, and the measurement feeds the layout back. The View
// on the outside reports what it always reported.
function Scroller({ className, contentContainerStyle, pinnedInScroller, ref, children, ...rest }: { className?: unknown; contentContainerStyle?: unknown; pinnedInScroller?: unknown; ref?: unknown; children?: ReactNode } & Record<string, unknown>) {
  const node = useElementRef(ref);
  const scroll = scrollOf(className);
  if (scroll === null) {
    if (isFixed(className)) {
      return <AnimatedView ref={node} className={className as string} bottomAnchored={isBottomAnchored(className)} {...rest}>{children}</AnimatedView>;
    }
    if (isSticky(className) && pinnedInScroller !== true) {
      return <StickyView ref={node} className={className as string} {...rest}>{children}</StickyView>;
    }
    return <View ref={node} className={className as string} {...rest}>{children}</View>;
  }
  const split = splitScroll(className);
  const content =
    Object.keys(split.content).length === 0
      ? contentContainerStyle
      : [contentContainerStyle, split.content];
  // A child the page wrote as sticky is a child the scroll view has to hold: the
  // browser pins it to the edge of the box that scrolls, and the scroll view does
  // the same for the children it is told about. Only a child of the box itself can
  // be one -- the index is the one of the ScrollView, not one of the tree.
  const parts = Children.toArray(children);
  const pinned = parts.flatMap((child, at) => (isValidElement(child) && isSticky((child.props as { className?: unknown }).className) ? [at] : []));
  const pinnedParts = parts.map((child, at) => (pinned.indexOf(at) >= 0 && isValidElement(child) && child.type === Box ? cloneElement(child as never, { pinnedInScroller: true } as never) : child));
  const view = (
    <ScrollView
      contentContainerStyle={content as never}
      horizontal={scroll === "horizontal"}
      keyboardShouldPersistTaps="handled"
      stickyHeaderIndices={scroll === "vertical" && pinned.length > 0 ? pinned : undefined}
      style={SCROLL_FILLS}
    >
      {pinnedParts}
    </ScrollView>
  );
  if (isFixed(className)) {
    return <AnimatedView ref={node} className={split.className} bottomAnchored={isBottomAnchored(className)} {...rest}>{view}</AnimatedView>;
  }
  return <View ref={node} className={split.className} {...rest}>{view}</View>;
}

// Only a Text may hold a string, so a block that holds text has to become one,
// and a button holds text the same way. Which strings are one text and which are
// items of their own is not decided here: see bifrost/runs.js. What that rule
// answers for a button is that an icon is an item and not part of the label.
// Measured on the login page of fonnso, with the whole of a button's children in
// one Text: the mark of Google took the 735 px of the box and the label was
// painted from x=834, past the 996 the button ends at -- and the same shape sat
// over the first letter of the button that sends the mail link.
function textRuns(children: ReactNode, className: unknown): ReactNode[] {
  return runsOf(
    Children.toArray(children),
    className,
    (child: unknown) => isValidElement(child) && child.type === Inline,
    (run: ReactNode[], at: number) => <Text key={"text-" + at}>{run}</Text>,
    (child: unknown) => isValidElement(child) && child.type === Fragment,
  );
}

function Box({ children, className, ...rest }: { children?: ReactNode; className?: unknown } & Record<string, unknown>) {
  return <Scroller className={className} {...rest}>{textRuns(children, className)}</Scroller>;
}

// An inline element -- a span, a label -- flows as text until it asks for a
// layout of its own: in a browser display:flex turns it into a box, and a Text
// cannot lay children out along an axis. Measured on the login form of the
// case: every <label> that held a field came out on one line with the field
// beside the label, because the map painted the label as a Text and the field
// as text inside it. When the class asks for flex the element is a Box, which
// is what the browser made of it.
function Inline({ className, children, ...rest }: { className?: unknown; children?: ReactNode } & Record<string, unknown>) {
  if (asksForFlex(className)) {
    return <Box className={className as string} {...rest}>{children}</Box>;
  }
  return <Text className={className as string} {...rest}>{children}</Text>;
}

function Anchor({ href, children, className, onPress, ref, ...rest }: { href?: string; children?: ReactNode; className?: unknown; onPress?: unknown; ref?: unknown } & Record<string, unknown>) {
  return (
    <Pressable
      ref={useElementRef(ref)}
      accessibilityRole="link"
      className={className as string}
      onPress={() => {
        if (typeof onPress === "function") {
          (onPress as () => void)();
        }
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
      {textRuns(children, className)}
    </Pressable>
  );
}

function Button({ children, type, className, onPress, ref, ...rest }: { children?: ReactNode; type?: unknown; className?: unknown; onPress?: unknown; ref?: unknown } & Record<string, unknown>) {
  // A button of the web paints its icon beside its label; a native press lays its
  // children in a column and the two came out one over the other. Measured on the
  // button that creates a discount of the app of the case.
  const row = Children.count(children) > 1 ? { flexDirection: "row" as const, alignItems: "center" as const, justifyContent: "center" as const, columnGap: 6 } : undefined;
  return <Pressable ref={useElementRef(ref)} accessibilityRole="button" className={className as string} style={row} onPress={usePress(type, onPress as (() => void) | undefined)} {...rest}>{textRuns(children, className)}</Pressable>;
}

// A form of the web app keeps its fields and submits them, because there is no
// browser to do it: the button of type="submit" inside asks it, and the fields
// answer with what they hold. The whole of it is in ./form, so that a button the
// app paints its own way still submits the form it is inside.
function FormTag({ onSubmit, children, ...rest }: { onSubmit?: unknown; children?: ReactNode } & Record<string, unknown>) {
  return <FormProvider onSubmit={onSubmit}><Box {...rest}>{children}</Box></FormProvider>;
}

// One field of a form: what the page set, and what somebody typed when the page
// let the field keep it. A page that reads the fields out of the event asks the
// form for them, and the answer is what a browser would have given it.
// A field is typed in, and the key that ends the typing is the one a browser
// calls Enter. A page writes that as onKeyDown, which is a name for the document
// and never fires here: measured on the app of the case, the composer of the chat
// answered Enter and the device did nothing, so the only way to send was the
// button.
//
// The event a page reads is given the two things it uses: the key, and the form
// it is inside, whose requestSubmit is the form of this map. That is why the
// composer needs no change to send with Enter.
//
// The type of a field is the browser's name for what the field is for, and a
// native control knows the same things but has no browser to read the name: a
// field of type="password" has to mask what is typed, and an email, a number, a
// phone and a url each bring the keyboard of what they ask for instead of the
// plain one. Measured on the login page of fonnso: the password painted in clear
// text on the device, and the email field opened the plain keyboard with its
// first letter capitalized. inputMode is the same thing said narrower -- it is
// only the keyboard -- and a page writes it when the field is text but the keys
// are not, like an otp code of digits, so it wins for the keyboard while type
// still wins for the masking.
function fieldControl(type: unknown, inputMode: unknown): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  if (type === "password") {
    // A browser masks a password, and it does not guess at it: no capitalization
    // and no correction.
    out.secureTextEntry = true;
    out.autoCapitalize = "none";
    out.autoCorrect = false;
  }
  const said = inputMode !== undefined && inputMode !== null ? inputMode : type;
  if (typeof said !== "string") {
    return out;
  }
  switch (said) {
    case "email":
      out.keyboardType = "email-address";
      out.autoCapitalize = "none";
      out.autoCorrect = false;
      break;
    case "number":
    case "decimal":
      out.keyboardType = "decimal-pad";
      break;
    case "numeric":
      out.keyboardType = "number-pad";
      break;
    case "tel":
      out.keyboardType = "phone-pad";
      break;
    case "url":
      out.keyboardType = "url";
      out.autoCapitalize = "none";
      out.autoCorrect = false;
      break;
    case "search":
      out.returnKeyType = "search";
      break;
  }
  return out;
}

function Field({ name, value, defaultValue, rows, type, inputMode, onKeyDown, onChange, ...rest }: Record<string, unknown>) {
  const form = useForm();
  const held = value === undefined ? defaultValue : value;
  if (form && typeof name === "string") {
    form.fields.set(name, held === undefined || held === null ? "" : String(held));
  }
  const enter = (event: unknown) => {
    if (typeof onKeyDown !== "function") {
      return;
    }
    (onKeyDown as (event: unknown) => void)({
      key: "Enter",
      shiftKey: false,
      metaKey: false,
      ctrlKey: false,
      altKey: false,
      preventDefault() {},
      stopPropagation() {},
      nativeEvent: event,
      target: null,
      currentTarget: { form: { requestSubmit: () => form?.submit() } },
    });
  };
  return (
    <TextInput
      defaultValue={value === undefined ? (defaultValue as string) : (value as string)}
      // A field of many rows wraps its text and stays single until it is sent,
      // which is what the rows of a textarea ask for; one of one row keeps the
      // behaviour of an input, whose Enter ends the typing.
      multiline={typeof rows === "number"}
      submitBehavior={typeof rows === "number" ? "submit" : "blurAndSubmit"}
      onSubmitEditing={onKeyDown === undefined ? undefined : enter}
      {...fieldControl(type, inputMode)}
      {...rest}
      onChangeText={(text: string) => {
        if (form && typeof name === "string") {
          form.fields.set(name, text);
        }
        // A page that keeps the value in its own state asks for it with
        // onChange, and a field that never answers leaves the state empty: the
        // page re-renders with the value it had, and the field looks dead --
        // measured, the search box of the files panel of the app typed and the
        // list never filtered. The event a page reads is the one it reads in a
        // browser: target.value, and the name the field was written with.
        if (typeof onChange === "function") {
          (onChange as (event: unknown) => void)({
            target: { value: text, name },
            currentTarget: { value: text, name },
            nativeEvent: { text },
          });
        }
      }}
    />
  );
}

// A page names the source of an image in two ways: a module it imported, which
// the bundler hands over as an asset, and a url the server answers. A browser
// resolves the url against the document it is in; a native client has no
// document, so the path is the origin of the app. Measured on the device: an
// imported logo and a rooted one both painted nothing -- a number is not a uri,
// and there was no server named by the path.
function imageSource(src: unknown): unknown {
  if (typeof src === "number") {
    return src;
  }
  if (typeof src === "object" && src !== null && typeof (src as { uri?: unknown }).uri === "string") {
    return src;
  }
  if (typeof src !== "string" || src === "") {
    return undefined;
  }
  if (src.charAt(0) === "/" && src.charAt(1) !== "/") {
    return { uri: ORIGIN + src };
  }
  return { uri: src };
}

// A picture of the web can be a drawing, and the image of a target is not one:
// the logo of the app of the case is an svg, and it painted nothing. The drawing
// is fetched and painted by the svg renderer, at the size the page asked for.
function imageUri(src: unknown): string | null {
  const source = imageSource(src) as { uri?: string } | number | undefined;
  if (source === undefined || typeof source === "number" || typeof source.uri !== "string") {
    return null;
  }
  return source.uri;
}

function sizeOf(value: unknown): number | undefined {
  if (typeof value === "number") {
    return value;
  }
  if (typeof value === "string") {
    const size = Number(value);
    return Number.isFinite(size) ? size : undefined;
  }
  return undefined;
}

function ImageTag({ src, alt, width, height, className, onLoad, ...rest }: { src?: unknown; alt?: string; width?: unknown; height?: unknown; className?: unknown; onLoad?: unknown } & Record<string, unknown>) {
  const uri = imageUri(src);
  if (uri !== null && /\.svg(\?|#|$)/i.test(uri)) {
    return <SvgUri uri={uri} width={sizeOf(width)} height={sizeOf(height)} accessibilityLabel={alt} />;
  }
  // A picture of the web that fades in when it is loaded takes the class that
  // hid it off the node it got in the event: there is no node here, and the class
  // that hid it would stay -- measured on the cards of a profile, "Cannot read
  // property 'remove' of undefined". The picture is painted without the hiding
  // class, and the handler still receives a node with a class list.
  const hidden = typeof className === "string" && /\bopacity-0\b/.test(className);
  const classes = hidden ? className.replace(/\bopacity-0\b/g, "") : className;
  const loaded = typeof onLoad === "function"
    ? (event: unknown) => {
        const node = (globalThis as { document?: { createElement?: (tag: string) => unknown } }).document?.createElement?.("img") ?? {};
        (onLoad as (event: unknown) => void)({ ...(event as Record<string, unknown>), target: node, currentTarget: node });
      }
    : undefined;
  return <Image source={imageSource(src) as never} accessibilityLabel={alt} className={classes as string} onLoad={loaded} {...rest} />;
}

// The type of a field is not only its keyboard: a checkbox and a radio are
// controls a browser draws, and painting them as text fields was worse than
// painting nothing -- measured on the device, a checkbox came out as an empty
// input and a radio as an input holding its own value. What is painted here is
// what a browser sends: a box that is checked, an event that carries
// target.checked, and a radio group that turns the others off by name.
const radioGroups = new Map<string, { selected: string | undefined; listeners: Set<() => void> }>();

function radioGroup(name: string): { selected: string | undefined; listeners: Set<() => void> } {
  let group = radioGroups.get(name);
  if (group === undefined) {
    group = { selected: undefined, listeners: new Set() };
    radioGroups.set(name, group);
  }
  return group;
}

// The value a control sends with a form when the page did not give it one: a
// browser sends "on" for a checkbox, and nothing for a radio without a value.
function controlValue(type: string, value: unknown): string {
  if (typeof value === "string") {
    return value;
  }
  if (typeof value === "number") {
    return String(value);
  }
  return type === "checkbox" ? "on" : "";
}

function controlEvent(type: string, checked: boolean, value: string, name: unknown): Record<string, unknown> {
  const target = { checked, value, name, type };
  return { target, currentTarget: target, nativeEvent: { checked, value } };
}

function Control({ type, checked, className, disabled, onPress, ...rest }: Record<string, unknown>) {
  // The class sizes the box, and a checkbox without one would be a mark with no
  // box around it: a browser draws a control even when the page styles nothing.
  const bare = className === undefined || className === null || className === "";
  return (
    <Pressable
      {...rest}
      accessibilityRole={type === "radio" ? "radio" : "checkbox"}
      accessibilityState={{ checked: checked === true, disabled: disabled === true }}
      className={className as string}
      disabled={disabled === true}
      onPress={onPress as () => void}
      style={bare ? styles.control : undefined}
    >
      <View pointerEvents="none" style={styles.center}>
        {checked === true ? <Text style={type === "radio" ? styles.dot : styles.check}>{type === "radio" ? "●" : "✓"}</Text> : null}
      </View>
    </Pressable>
  );
}

function CheckBox({ checked, defaultChecked, onChange, name, value, disabled, className, ...rest }: Record<string, unknown>) {
  const form = useForm();
  const [own, setOwn] = useState(defaultChecked === true);
  const on = checked === undefined ? own : checked === true;
  const sent = controlValue("checkbox", value);
  if (form !== null && typeof name === "string") {
    if (on) {
      form.fields.set(name, sent);
    } else {
      form.fields.delete(name);
    }
  }
  const press = () => {
    const next = !on;
    if (checked === undefined) {
      setOwn(next);
    }
    if (typeof onChange === "function") {
      (onChange as (event: unknown) => void)(controlEvent("checkbox", next, sent, name));
    }
  };
  return <Control type="checkbox" checked={on} className={className} disabled={disabled} onPress={press} {...rest} />;
}

// A radio group is the name its fields share: a browser turns the others off
// when one is chosen, and a page that keeps the choice in its own state does the
// same through checked. An uncontrolled group keeps the choice here, because no
// page is holding it.
function Radio({ checked, defaultChecked, onChange, name, value, disabled, className, ...rest }: Record<string, unknown>) {
  const form = useForm();
  const sent = controlValue("radio", value);
  const group = checked === undefined && typeof name === "string" ? radioGroup(name) : undefined;
  const selected = useSyncExternalStore(
    (listener) => {
      if (group === undefined) {
        return () => {};
      }
      group.listeners.add(listener);
      return () => {
        group.listeners.delete(listener);
      };
    },
    () => (group === undefined ? undefined : group.selected),
    () => undefined,
  );
  const [own, setOwn] = useState(defaultChecked === true);
  // The default of a group is written down the first time one of its fields is
  // painted, so the field that comes before it in the tree already knows.
  if (group !== undefined && group.selected === undefined && defaultChecked === true) {
    group.selected = sent;
  }
  const on = checked !== undefined ? checked === true : group !== undefined ? selected === sent : own;
  if (form !== null && typeof name === "string" && on) {
    form.fields.set(name, sent);
  }
  const press = () => {
    if (on) {
      return;
    }
    if (checked === undefined) {
      if (group !== undefined) {
        group.selected = sent;
        for (const listener of group.listeners) {
          listener();
        }
      } else {
        setOwn(true);
      }
    }
    if (typeof onChange === "function") {
      (onChange as (event: unknown) => void)(controlEvent("radio", true, sent, name));
    }
  };
  return <Control type="radio" checked={on} className={className} disabled={disabled} onPress={press} {...rest} />;
}

// A browser draws a field of type="date" with a calendar and one of type="time"
// with a clock; both hand the page a string, YYYY-MM-DD and HH:MM. Measured on
// the device: each opened the keyboard and painted its own string. The calendar
// and the clock are the ones of the device, and the value is the one a browser
// gives it.
const DATE_VALUE = /^(\d{4})-(\d{2})-(\d{2})$/;
const TIME_VALUE = /^(\d{2}):(\d{2})$/;

function dateOf(value: unknown): Date | null {
  if (typeof value !== "string") {
    return null;
  }
  const match = DATE_VALUE.exec(value);
  if (match === null) {
    return null;
  }
  const date = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
  return Number.isNaN(date.getTime()) ? null : date;
}

// A clock has no date: the picker only needs one to carry the time, and today is
// as good as any other day for that.
function timeOf(value: unknown): Date | null {
  if (typeof value !== "string") {
    return null;
  }
  const match = TIME_VALUE.exec(value);
  if (match === null) {
    return null;
  }
  const date = new Date();
  date.setHours(Number(match[1]), Number(match[2]), 0, 0);
  return Number.isNaN(date.getTime()) ? null : date;
}

function isoOf(date: Date): string {
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return date.getFullYear() + "-" + month + "-" + day;
}

function hourOf(date: Date): string {
  return String(date.getHours()).padStart(2, "0") + ":" + String(date.getMinutes()).padStart(2, "0");
}

function clockOf(mode: string, value: unknown): Date | null {
  return mode === "time" ? timeOf(value) : dateOf(value);
}

function valueOf(mode: string, date: Date): string {
  return mode === "time" ? hourOf(date) : isoOf(date);
}

// What the field paints: a browser shows the date and the clock of the locale,
// not the string it hands the page.
function labelOf(mode: string, date: Date): string {
  return mode === "time" ? date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : date.toLocaleDateString();
}

function ClockField({ mode, type, value, defaultValue, onChange, name, min, max, disabled, className, ...rest }: Record<string, unknown>) {
  const kind = mode === "time" ? "time" : "date";
  const form = useForm();
  const [own, setOwn] = useState<Date | null>(() => clockOf(kind, defaultValue));
  const [open, setOpen] = useState(false);
  const current = value === undefined ? own : clockOf(kind, value);
  const sent = current === null ? "" : valueOf(kind, current);
  if (form !== null && typeof name === "string") {
    form.fields.set(name, sent);
  }
  const picked = (event: unknown, date: Date) => {
    // The dialog of Android is a dialog: picking it or dismissing it is the whole
    // visit. The spinner of iOS stays until the page closes it.
    if (Platform.OS === "android") {
      setOpen(false);
    }
    if (value === undefined) {
      setOwn(date);
    }
    const next = valueOf(kind, date);
    if (form !== null && typeof name === "string") {
      form.fields.set(name, next);
    }
    if (typeof onChange !== "function") {
      return;
    }
    (onChange as (event: unknown) => void)({
      target: { value: next, name, type: kind },
      currentTarget: { value: next, name, type: kind },
      nativeEvent: { text: next },
    });
  };
  const picker = (
    <DateTimePicker
      value={current === null ? new Date() : current}
      mode={kind as "date" | "time"}
      minimumDate={clockOf(kind, min) ?? undefined}
      maximumDate={clockOf(kind, max) ?? undefined}
      onValueChange={picked}
      onDismiss={() => setOpen(false)}
    />
  );
  return (
    <>
      <Pressable
        {...rest}
        accessibilityRole="button"
        className={className as string}
        disabled={disabled === true}
        onPress={() => setOpen(true)}
        style={className === undefined || className === null || className === "" ? styles.clock : undefined}
      >
        <Text>{current === null ? " " : labelOf(kind, current)}</Text>
      </Pressable>
      {open === false ? null : Platform.OS === "android" ? picker : (
        <Modal transparent animationType="fade" visible onRequestClose={() => setOpen(false)}>
          <Pressable style={styles.backdrop} onPress={() => setOpen(false)}>
            <View style={styles.sheet}>
              {picker}
              <Pressable onPress={() => setOpen(false)}>
                <Text>Done</Text>
              </Pressable>
            </View>
          </Pressable>
        </Modal>
      )}
    </>
  );
}

// A browser draws a field of type="range" as a slider and hands the page the
// value as a string. Measured on the device: the field was a text field of no
// height, and the value was typed instead of dragged. The track is the one of the
// device; what the page reads is the string a browser gives it.
function numberOf(value: unknown, fallback: number): number {
  const number = typeof value === "number" ? value : typeof value === "string" && value.trim() !== "" ? Number(value) : NaN;
  return Number.isFinite(number) ? number : fallback;
}

const SliderRoot = styled(Slider);

function RangeField({ value, defaultValue, onChange, onInput, name, min, max, step, disabled, className, ...rest }: Record<string, unknown>) {
  const form = useForm();
  const low = numberOf(min, 0);
  const high = numberOf(max, 100);
  const stride = step === "any" ? 0 : numberOf(step, 1);
  // Without a value, a browser starts at the middle of the range.
  const middle = low + (high - low) / 2;
  const [own, setOwn] = useState(() => (defaultValue === undefined ? middle : numberOf(defaultValue, middle)));
  const wanted = value === undefined ? own : numberOf(value, middle);
  const current = Math.min(Math.max(wanted, low), high);
  const sent = String(current);
  if (form !== null && typeof name === "string") {
    form.fields.set(name, sent);
  }
  // A browser fires input and change while the thumb moves; a native slider fires
  // one callback, and the two names a page may use read it.
  const changed = (next: number) => {
    if (value === undefined) {
      setOwn(next);
    }
    if (form !== null && typeof name === "string") {
      form.fields.set(name, String(next));
    }
    const event = {
      target: { value: String(next), name, type: "range" },
      currentTarget: { value: String(next), name, type: "range" },
      nativeEvent: { text: String(next) },
    };
    for (const handler of [onInput, onChange]) {
      if (typeof handler === "function") {
        (handler as (event: unknown) => void)(event);
      }
    }
  };
  return (
    <SliderRoot
      {...rest}
      accessibilityRole="adjustable"
      className={className as string}
      disabled={disabled === true}
      maximumValue={high}
      minimumValue={low}
      step={stride}
      value={current}
      onValueChange={changed}
      style={className === undefined || className === null || className === "" ? styles.slider : undefined}
    />
  );
}

// A file is not a value of the document: a browser opens the file dialog of the
// system and keeps the file for the form. Measured on the device: an
// <input type="file"> was a text field, so there was no way to choose a file at
// all. The picker is the one of the device; the form keeps the part that React
// Native's own uploader reads, and ./net sends a multipart request with one.
type Picked = { uri: string; name: string; type: string; size?: number };

function accepted(accept: unknown): string | string[] {
  if (typeof accept !== "string" || accept.trim() === "") {
    return "*/*";
  }
  const list = accept
    .split(",")
    .map((one) => one.trim())
    .filter((one) => one !== "");
  return list.length === 1 ? list[0] : list;
}

function FileField({ name, multiple, accept, disabled, className, onChange, value, type, ...rest }: Record<string, unknown>) {
  const form = useForm();
  const [files, setFiles] = useState<Picked[]>([]);
  const uploads = files.map((one) => ({ uri: one.uri, name: one.name, type: one.type }));
  if (form !== null && typeof name === "string") {
    form.files.set(name, uploads);
  }
  const pick = async () => {
    let result;
    try {
      result = await DocumentPicker.getDocumentAsync({
        type: accepted(accept),
        multiple: multiple === true,
        copyToCacheDirectory: true,
      });
    } catch (error) {
      console.warn("bifrost: the picker of the device failed: " + String(error));
      return;
    }
    if (result.canceled) {
      return;
    }
    const picked: Picked[] = result.assets.map((asset) => ({
      uri: asset.uri,
      name: asset.name,
      type: asset.mimeType ?? "application/octet-stream",
      size: asset.size,
    }));
    setFiles(picked);
    if (form !== null && typeof name === "string") {
      form.files.set(name, picked.map((one) => ({ uri: one.uri, name: one.name, type: one.type })));
    }
    if (typeof onChange !== "function") {
      return;
    }
    // The event a page reads has the files it chose, and the name of the first
    // one where a browser puts it.
    const said = picked[0] === undefined ? "" : picked[0].name;
    (onChange as (event: unknown) => void)({
      target: { files: picked, value: said, name, type: "file" },
      currentTarget: { files: picked, value: said, name, type: "file" },
      nativeEvent: { files: picked },
    });
  };
  const bare = className === undefined || className === null || className === "";
  return (
    <Pressable
      {...rest}
      accessibilityRole="button"
      className={className as string}
      disabled={disabled === true}
      onPress={() => {
        void pick();
      }}
      style={bare ? styles.file : undefined}
    >
      <Text>{files.length === 0 ? "Choose a file" : files.map((one) => one.name).join(", ")}</Text>
    </Pressable>
  );
}

// The type of the input decides which control it is; everything else is a field
// of text.
function InputTag(props: Record<string, unknown>) {
  if (props.type === "date" || props.type === "time") {
    return <ClockField mode={props.type} {...props} />;
  }
  if (props.type === "range") {
    return <RangeField {...props} />;
  }
  if (props.type === "file") {
    return <FileField {...props} />;
  }
  if (props.type === "checkbox") {
    return <CheckBox {...props} />;
  }
  if (props.type === "radio") {
    return <Radio {...props} />;
  }
  return <Field {...props} />;
}

// A browser opens a select and lists the options it holds; a native target has
// neither, and a tag this table does not paint takes the whole screen down --
// measured: one <select> in a form and the render stopped at "View config getter
// callback for component option must be a function". The list is painted with
// what every target has: the chosen option in a pressable, and the rest in a
// sheet that covers the page while it is open.
type Choice = { value: string; label: string; disabled: boolean };

function textOf(children: ReactNode): string {
  let text = "";
  for (const child of Children.toArray(children)) {
    if (typeof child === "string" || typeof child === "number") {
      text += String(child);
      continue;
    }
    if (isValidElement(child)) {
      text += textOf((child.props as { children?: ReactNode }).children);
    }
  }
  return text;
}

// An option without a value is worth its label, which is what a browser sends;
// an option group is not an option, so its children are read as if they were
// here.
function choicesOf(children: ReactNode): Choice[] {
  const choices: Choice[] = [];
  for (const child of Children.toArray(children)) {
    if (!isValidElement(child)) {
      continue;
    }
    const props = child.props as { children?: ReactNode; value?: unknown; disabled?: unknown };
    if (child.type === OptionGroup) {
      choices.push(...choicesOf(props.children));
      continue;
    }
    if (child.type !== Option) {
      continue;
    }
    const label = textOf(props.children);
    choices.push({
      value: typeof props.value === "string" ? props.value : label,
      label,
      disabled: props.disabled === true,
    });
  }
  return choices;
}

function Option() {
  return null;
}

function OptionGroup() {
  return null;
}

function SingleSelect({ children, className, value, defaultValue, onChange, name, disabled, multiple, ...rest }: { children?: ReactNode; className?: unknown; value?: unknown; defaultValue?: unknown; onChange?: unknown; name?: unknown; disabled?: unknown; multiple?: unknown } & Record<string, unknown>) {
  const form = useForm();
  const choices = choicesOf(children);
  const first = choices.find((choice) => !choice.disabled);
  // Without a value and without a default, a browser selects the first option
  // that is not disabled, and a field that painted nothing would look empty
  // where the web sends something.
  const [own, setOwn] = useState(() => {
    const initial = Array.isArray(defaultValue) ? defaultValue[0] : defaultValue;
    if (initial !== undefined) {
      return initial === null ? "" : String(initial);
    }
    return first === undefined ? "" : first.value;
  });
  const [open, setOpen] = useState(false);
  const wanted = Array.isArray(value) ? value[0] : value;
  const chosen = wanted === undefined ? own : wanted === null ? "" : String(wanted);
  const current = choices.find((choice) => choice.value === chosen);
  if (form !== null && typeof name === "string") {
    form.fields.set(name, chosen);
  }
  // The event is the one a page reads from a browser: target.value and the name
  // the field was written with.
  const pick = (choice: Choice) => {
    setOpen(false);
    if (value === undefined) {
      setOwn(choice.value);
    }
    if (form !== null && typeof name === "string") {
      form.fields.set(name, choice.value);
    }
    if (typeof onChange !== "function") {
      return;
    }
    (onChange as (event: unknown) => void)({
      target: { value: choice.value, name },
      currentTarget: { value: choice.value, name },
      nativeEvent: { text: choice.value },
    });
  };
  return (
    <>
      <Pressable
        {...rest}
        accessibilityRole="button"
        accessibilityState={{ disabled: disabled === true, expanded: open }}
        className={className as string}
        disabled={disabled === true}
        onPress={() => setOpen(true)}
      >
        <Text>{current === undefined ? "" : current.label}</Text>
      </Pressable>
      <Modal transparent animationType="fade" visible={open} onRequestClose={() => setOpen(false)}>
        <Pressable style={styles.backdrop} onPress={() => setOpen(false)}>
          <View style={styles.sheet}>
            {choices.map((choice, at) => (
              <Pressable
                key={at}
                accessibilityRole="button"
                accessibilityState={{ disabled: choice.disabled, selected: choice.value === chosen }}
                disabled={choice.disabled}
                onPress={() => pick(choice)}
                style={styles.choice}
              >
                <Text>{choice.label}</Text>
              </Pressable>
            ))}
          </View>
        </Pressable>
      </Modal>
    </>
  );
}

// A select that takes more than one value is a list a browser paints in place,
// with every option visible and each one toggled on its own; there is no sheet to
// open. The form carries a list per name, because this is the only control whose
// name can answer with more than one value.
function selectedOf(value: unknown, choices: Choice[]): string[] {
  const said = Array.isArray(value) ? value.map((one) => String(one)) : typeof value === "string" ? value.split(",") : [];
  return choices.filter((choice) => said.indexOf(choice.value) >= 0).map((choice) => choice.value);
}

function MultipleSelect({ children, className, value, defaultValue, onChange, name, disabled, ...rest }: { children?: ReactNode } & Record<string, unknown>) {
  const form = useForm();
  const choices = choicesOf(children);
  const [own, setOwn] = useState<string[]>(() => selectedOf(defaultValue, choices));
  const chosen = value === undefined ? own : selectedOf(value, choices);
  if (form !== null && typeof name === "string") {
    form.lists.set(name, chosen);
  }
  // What a browser sends is a list in the order the options are written, and the
  // page reads it as the value of the event.
  const toggle = (choice: Choice) => {
    const on = chosen.indexOf(choice.value) >= 0;
    const next = choices
      .filter((one) => (one.value === choice.value ? !on : chosen.indexOf(one.value) >= 0))
      .map((one) => one.value);
    if (value === undefined) {
      setOwn(next);
    }
    if (form !== null && typeof name === "string") {
      form.lists.set(name, next);
    }
    if (typeof onChange !== "function") {
      return;
    }
    const selectedOptions = next.map((one) => ({ value: one, selected: true }));
    (onChange as (event: unknown) => void)({
      target: { value: next, selectedOptions, name, type: "select-multiple" },
      currentTarget: { value: next, selectedOptions, name, type: "select-multiple" },
      nativeEvent: { text: next.join(",") },
    });
  };
  return (
    <View {...rest} className={className as string} style={className === undefined || className === null || className === "" ? styles.list : undefined}>
      {choices.map((choice, at) => {
        const on = chosen.indexOf(choice.value) >= 0;
        return (
          <Pressable
            key={at}
            accessibilityRole="checkbox"
            accessibilityState={{ checked: on, disabled: choice.disabled || disabled === true }}
            disabled={choice.disabled || disabled === true}
            onPress={() => toggle(choice)}
            style={[styles.option, on ? styles.optionOn : undefined]}
          >
            <Text>{choice.label}</Text>
          </Pressable>
        );
      })}
    </View>
  );
}

// The two selects are different controls, not one with a flag, so the tag picks
// the one the page asked for.
function Select(props: Record<string, unknown>) {
  return props.multiple === true ? <MultipleSelect {...props} /> : <SingleSelect {...props} />;
}

// A details is a box that folds: a browser paints its summary as the handle and
// hides the rest until it is opened. Measured on the device: the body was always
// painted while the page said it was closed. The summary is the first child that
// is one, and everything else is the body; the marker is the one of the browser.
function Summary({ children, className }: { children?: ReactNode; className?: unknown }) {
  return <Inline className={className}>{children}</Inline>;
}

function Details({ children, open, defaultOpen, onToggle, className, ...rest }: { children?: ReactNode } & Record<string, unknown>) {
  const [own, setOwn] = useState(defaultOpen === true);
  const shown = open === undefined ? own : open === true;
  const parts = Children.toArray(children);
  const head = parts.find((child) => isValidElement(child) && child.type === Summary);
  const body = parts.filter((child) => child !== head);
  const headProps = head !== undefined && isValidElement(head) ? (head.props as { children?: ReactNode; className?: unknown }) : undefined;
  const summary = headProps === undefined ? "Details" : headProps.children;
  const summaryClass = headProps === undefined ? undefined : headProps.className;
  const toggle = () => {
    const next = !shown;
    if (open === undefined) {
      setOwn(next);
    }
    if (typeof onToggle !== "function") {
      return;
    }
    (onToggle as (event: unknown) => void)({
      target: { open: next },
      currentTarget: { open: next },
      nativeEvent: { open: next },
    });
  };
  return (
    <View {...rest} className={className as string}>
      <Pressable
        accessibilityRole="button"
        accessibilityState={{ expanded: shown }}
        className={summaryClass as string}
        onPress={toggle}
        style={summaryClass === undefined || summaryClass === null || summaryClass === "" ? styles.summary : undefined}
      >
        {textRuns([shown ? "▾ " : "▸ ", summary], summaryClass)}
      </Pressable>
      {shown ? textRuns(body, undefined) : null}
    </View>
  );
}

function Rule() {
  return <View style={{ height: 1, backgroundColor: "#e9dfd2" }} />;
}

// An icon of the web is an svg, and the target has no vector tags of its own: an
// app had to ship a table of them in host-extra, and every app that drew one
// wrote the same table. Measured on the device, an <svg> warned "no native
// counterpart for <svg>" and painted nothing. react-native-svg is part of the
// runtime of Expo Go, so the drawing is painted here.
//
// The class of the root is the box of the drawing: h-4 w-4 sizes a View and the
// svg fills it, and the colour travels as currentColor does in a browser -- from
// the class of the icon or from the box that holds it.
function flatStyle(style: unknown): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const one of Array.isArray(style) ? style : [style]) {
    if (one !== null && typeof one === "object") {
      Object.assign(out, one);
    }
  }
  return out;
}

// stroke-width is how a string of HTML spells an attribute and strokeWidth is
// how React does; both name the same thing in a drawing. What belongs to the
// document and not to the drawing is left out.
function svgProps(props: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [name, value] of Object.entries(props)) {
    if (name === "className" || name === "xmlns" || name === "xmlnsXlink" || name === "aria-hidden" || name === "focusable") {
      continue;
    }
    const key = name.indexOf("-") < 0 ? name : name.replace(/-(.)/g, (_, letter: string) => letter.toUpperCase());
    out[key] = value;
  }
  return out;
}

function SvgCanvas({ style, children, ...rest }: { style?: unknown; children?: ReactNode } & Record<string, unknown>) {
  const carried = useInheritedColor();
  const flat = flatStyle(style);
  const color = typeof flat.color === "string" ? flat.color : carried;
  const sized = flat.width !== undefined || flat.height !== undefined;
  const props = svgProps(rest);
  return (
    <View style={style}>
      {createElement(Svg as never, {
        ...props,
        width: sized ? "100%" : props.width,
        height: sized ? "100%" : props.height,
        color,
      }, children)}
    </View>
  );
}

// Every tag of a drawing takes the props of a drawing and nothing of the
// document; the root is the one that also owns the box.
function shape(component: unknown) {
  return function Shape(props: Record<string, unknown>) {
    return createElement(component as never, svgProps(props));
  };
}

const SvgRoot = styled(SvgCanvas);
const SvgCircle = shape(Circle);
const SvgClipPath = shape(ClipPath);
const SvgDefs = shape(Defs);
const SvgEllipse = shape(Ellipse);
const SvgG = shape(G);
const SvgLine = shape(Line);
const SvgLinearGradient = shape(LinearGradient);
const SvgMask = shape(Mask);
const SvgPath = shape(Path);
const SvgPolygon = shape(Polygon);
const SvgPolyline = shape(Polyline);
const SvgRadialGradient = shape(RadialGradient);
const SvgRect = shape(Rect);
const SvgStop = shape(Stop);
const SvgSymbol = shape(SymbolTag);
const SvgUse = shape(Use);

function Nothing() {
  return null;
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, backgroundColor: "rgba(0, 0, 0, 0.4)", justifyContent: "flex-end" },
  sheet: { backgroundColor: "#ffffff", borderTopLeftRadius: 12, borderTopRightRadius: 12, paddingVertical: 8 },
  choice: { paddingHorizontal: 20, paddingVertical: 14 },
  control: { width: 18, height: 18 },
  center: { position: "absolute", top: 0, right: 0, bottom: 0, left: 0, alignItems: "center", justifyContent: "center" },
  check: { fontSize: 12, lineHeight: 14 },
  dot: { fontSize: 8, lineHeight: 10 },
  clock: { paddingHorizontal: 12, paddingVertical: 8 },
  slider: { width: "100%", height: 40 },
  list: { borderWidth: 1, borderColor: "#d4d4d4", borderRadius: 6 },
  option: { paddingHorizontal: 12, paddingVertical: 8 },
  optionOn: { backgroundColor: "#dbeafe" },
  summary: { paddingVertical: 6 },
  file: { paddingHorizontal: 12, paddingVertical: 8, borderWidth: 1, borderColor: "#d4d4d4", borderRadius: 6 },
});

const NAMED: Record<string, unknown> = {
  a: Anchor,
  button: Button,
  form: FormTag,
  hr: Rule,
  img: ImageTag,
  input: InputTag,
  optgroup: OptionGroup,
  option: Option,
  select: Select,
  textarea: Field,
};
for (const name of BOXES) {
  NAMED[name] = Box;
}
for (const name of TEXTS) {
  NAMED[name] = Inline;
}
for (const name of HIDDEN) {
  NAMED[name] = Nothing;
}
// The tags of a drawing, in the case React spells them: a page writes
// linearGradient and clipPath, and the runtime hands the name as it was written.
for (const [name, component] of Object.entries({
  circle: SvgCircle,
  clipPath: SvgClipPath,
  defs: SvgDefs,
  ellipse: SvgEllipse,
  g: SvgG,
  line: SvgLine,
  linearGradient: SvgLinearGradient,
  mask: SvgMask,
  path: SvgPath,
  polygon: SvgPolygon,
  polyline: SvgPolyline,
  radialGradient: SvgRadialGradient,
  rect: SvgRect,
  stop: SvgStop,
  svg: SvgRoot,
  symbol: SvgSymbol,
  use: SvgUse,
})) {
  NAMED[name] = component;
}
// A details and its summary are one control, and the box of each is not what the
// page meant: the summary is the handle and the rest waits.
NAMED.details = Details;
NAMED.summary = Summary;

// The interaction of a page is written the way a browser writes it: onClick. A
// native view is pressed, and React Native answers onPress -- the browser's name
// is dropped in silence. Measured on the login page of fonnso: the button that
// opens the link of the mail answered onClick and did nothing on the device, and
// so did each of the seventy-eight onClick of the app.
//
// It is kept a press and nothing else: the button that sends a form asks the
// form with ./form, which needs the element it is inside and so cannot happen
// here.
//
// The other thing answered here is the HTML a page injects. It is a string of
// tags, and the only reader of it is a browser; a tag this table paints is
// painted, and its children are read the same way. Measured on fonnso: the body
// of every message of the assistant is markdown compiled to HTML, and the bubble
// came out empty.
//
// And the state of an element, which a page writes as a data attribute and a
// stylesheet picks up with a data-[...] variant. The compiler reads those from a
// prop called dataSet, so an attribute that travels with its own name reaches
// nothing: measured on the case, every placement of the sheet of the app -- the
// side it comes from, its height, how far it slides -- lived under a
// data-[side=right] variant, and the panel came out at the top of the list in
// the order the tree happened to have it, with none of it applied. The name is
// turned the way the compiler spells it: a dash and the letter after it, in
// camel case.
//
// And the place of an element that a page worked out by measuring the page: see
// placed, below.
// A row of the web spaces its children with a rule that names the sibling of a
// child -- space-x-4 is a margin on everything but the first -- and a target has
// no siblings to name: the classes are a style of one box. The space the classes
// ask for is a gap of that box. Measured on the tabs of the discounts page of the
// app of the case: two buttons of a row came out touching.
function spaced(className: unknown): Record<string, number> | undefined {
  if (typeof className !== "string") {
    return undefined;
  }
  const x = /(?:^|\s)space-x-(\d+(?:\.\d+)?)(?:\s|$)/.exec(className);
  const y = /(?:^|\s)space-y-(\d+(?:\.\d+)?)(?:\s|$)/.exec(className);
  if (x === null && y === null) {
    return undefined;
  }
  const gap: Record<string, number> = {};
  if (x !== null) {
    gap.columnGap = Number(x[1]) * 4;
  }
  if (y !== null) {
    gap.rowGap = Number(y[1]) * 4;
  }
  return gap;
}

export function nativeProps(props: unknown, tag?: string): unknown {
  if (props === null || typeof props !== "object") {
    return props;
  }
  const given = { ...(props as Record<string, unknown>) };
  for (const name of Object.keys(given)) {
    if (!name.startsWith("data-") || name.length === 5) {
      continue;
    }
    const key = name.slice(5).replace(/-(.)/g, (_, letter: string) => letter.toUpperCase());
    const state = (given.dataSet ?? {}) as Record<string, unknown>;
    state[key] = given[name];
    given.dataSet = state;
    delete given[name];
  }
  const inner = given.dangerouslySetInnerHTML as { __html?: unknown } | undefined;
  if (inner !== undefined) {
    delete given.dangerouslySetInnerHTML;
    if (typeof inner?.__html === "string") {
      given.children = painted(inner.__html);
    }
  }
  // A handler of the web reads the event it is given -- the element it happened
  // in, the pointer, the default it can prevent -- and a press of the target
  // carries none of those. It is handed the shape a handler reads, with the node
  // of the shim as the element: measured on the arrow of a select, "Cannot read
  // property 'currentTarget' of undefined" stopped the first menu the app opened.
  const pressEvent = (event: unknown) => {
    const node = (globalThis as { document?: { createElement?: (tag: string) => unknown } }).document?.createElement?.("button") ?? {};
    const given = event !== null && typeof event === "object" ? (event as Record<string, unknown>) : {};
    return {
      ...given,
      target: given.target ?? node,
      currentTarget: given.currentTarget ?? node,
      button: given.button ?? 0,
      ctrlKey: given.ctrlKey ?? false,
      metaKey: given.metaKey ?? false,
      shiftKey: given.shiftKey ?? false,
      altKey: given.altKey ?? false,
      preventDefault: () => undefined,
      stopPropagation: () => undefined,
      nativeEvent: given.nativeEvent ?? { button: 0, pointerType: "touch" },
    };
  };
  if (typeof given.onClick === "function") {
    const click = given.onClick as (event: unknown) => void;
    delete given.onClick;
    const pressed = given.onPress as ((event: unknown) => void) | undefined;
    given.onPress = (event: unknown) => {
      const shaped = pressEvent(event);
      pressed?.(shaped);
      click(shaped);
    };
  }
  // A menu of the web opens on the pointer going down, not on the click that
  // follows: the trigger of a select listens to onPointerDown, and a press that
  // only answered onClick left the menu closed. Measured on the language toggle
  // of the app of the case. The event a browser hands it is one the trigger can
  // read -- the left button, no ctrl -- and the press of a target stands for it.
  if (given.onPress === undefined && typeof given.onPointerDown === "function") {
    const down = given.onPointerDown as (event: unknown) => void;
    delete given.onPointerDown;
    given.onPress = (event: unknown) => {
      down(pressEvent({ ...pressEvent(event), button: 0, ctrlKey: false, metaKey: false, shiftKey: false, altKey: false, pointerType: "touch" }));
    };
  } else {
    delete given.onPointerDown;
  }
  // A page places a popup by measuring the page, and what it measured arrives as
  // a transform of the style: react-native reads that string too, and refuses one
  // whose numbers are not numbers. Measured on the user menu of fonnso: a target
  // that cannot answer a client rect hands over translate(NaNpx, NaNpx) and the
  // renderer throws "Transform with key translate must be an string with 1 or 2
  // parameters, found 0" -- the whole screen, black. A number that is not a
  // number is not a place, so the declaration goes and the box stays where the
  // flow put it, which is the only place a target without a browser's
  // measurements can honestly name.
  if (given.style !== undefined) {
    given.style = placed(given.style);
  }
  const gap = spaced(given.className);
  if (gap !== undefined) {
    given.style = given.style === undefined ? gap : [given.style, gap];
  }
  // The tag the page wrote an element of its own as. It arrives only when the
  // element is painted by a component of this table -- a name this table does not
  // know is left as it was, and asking here again would say so twice.
  if (typeof tag === "string") {
    tagged(given, tag);
  }
  return given;
}

// A node of the HTML, as the component that paints its tag. A tag this table
// does not paint keeps its children: the text of a paragraph is worth more than
// nothing, and a name react-native does not know would take the render down.
function nodeOf(node: unknown, key: string): unknown {
  if (typeof node === "string") {
    return node;
  }
  const element = node as { tag: string; props: Record<string, string>; children: unknown[] };
  const children = paintedNodes(element.children, key + ".");
  // A paragraph of the HTML is the block a browser paints: the text of a post
  // came out as one run when the tag was painted as the inline one a page writes
  // by hand. Measured on the body of a post of the app of the case.
  const component = element.tag === "p" ? Box : hostComponent(element.tag);
  if (component === undefined) {
    return createElement(Fragment, { key }, ...(children as []));
  }
  return createElement(component as never, { key, ...htmlProps(element.props, element.tag) }, ...(children as []));
}

function paintedNodes(nodes: unknown[], prefix: string): unknown[] {
  return nodes.map((node, index) => nodeOf(node, prefix + index));
}

// The classes of the HTML are the classes of the app: class is how a browser
// spells what nativewind reads as className, and the stylesheet the app
// already wrote -- .md, .code, .tok-keyword -- is the one that has to
// reach it. Anything else keeps its name and its value.
//
// The class of the tag comes before the element's own classes, so a rule of the
// app that names a class still wins over one that names the tag, which is the
// order a browser settles them in.
function htmlProps(props: Record<string, string>, tag?: string): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [name, value] of Object.entries(props)) {
    if (name === "class") {
      out.className = value;
      continue;
    }
    if (name === "dangerouslySetInnerHTML") {
      continue;
    }
    out[name] = value;
  }
  return tagged(out, tag);
}

// An element also carries a class named after the tag a page wrote it as, and
// that class is what the stylesheet rewrite of ./selectors hangs a rule written
// for a browser on: a rule written .md p becomes .md .native-p, because a native
// view has no tag of its own to name. The class is inert on its own: it matches
// nothing until a rule names it, so an app that never writes an element selector
// never sees it.
export function tagClass(tag: string): string {
  return "native-" + tag;
}

function tagged(out: Record<string, unknown>, tag?: string): Record<string, unknown> {
  if (tag === undefined) {
    return out;
  }
  const own = out.className;
  out.className = typeof own === "string" && own !== "" ? tagClass(tag) + " " + own : tagClass(tag);
  return out;
}

// The parser is asked for the first time a page injects HTML, and never by a
// page that does not.
function painted(html: string): unknown[] {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const { parseHTML } = require("./html") as { parseHTML(html: string): unknown[] };
  return paintedNodes(parseHTML(html), "html");
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

const nativeJSXRuntime = `import * as devRuntime from "react/jsx-dev-runtime";
import * as runtime from "react/jsx-runtime";

// This module is evaluated before the app's own runtime is ready, because the
// compiler needs it to build the first element. It must therefore pull nothing
// in by itself: importing react-native here would land it ahead of the URL and
// TextEncoder patches expo installs, and the app would die before it renders.
type Host = {
  hostComponent(name: string): unknown;
  droppedProps(name: string, props: unknown): string[];
  nativeProps(props: unknown, tag?: string): unknown;
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
    if (typeof type === "string") {
      if (__DEV__) {
        warnDropped(type, rest[0]);
      }
      const painted = component(type);
      return (create as (type: unknown, ...rest: unknown[]) => unknown)(
        painted,
        loaded().nativeProps(rest[0], typeof painted === "string" ? undefined : type),
        ...rest.slice(1),
      );
    }
    return (create as (type: unknown, ...rest: unknown[]) => unknown)(type, ...rest);
  };

export const jsx = wrap(runtime.jsx);
export const jsxs = wrap(runtime.jsxs);
// The runtime of a development build is a module of its own: react/jsx-runtime
// has no jsxDEV, and wrapping the one that is not there is an element built by
// a function that does not exist. Measured on the app of the case: the language
// select of the navbar stopped the first screen with "undefined is not a
// function".
export const jsxDEV = wrap(devRuntime.jsxDEV);
export const Fragment = runtime.Fragment;
`

var nativeProjectFiles = []nativeFile{
	{"tsconfig.json", nativeTSConfig},
	{"metro.config.js", nativeMetroConfig},
	{"bifrost/css-warnings.js", nativeCSSWarnings},
	{"bifrost/native-internal.js", nativeNativeInternal},
	{"bifrost/row-direction.js", nativeRowDirection},
	{"bifrost/runs.js", nativeRuns},
	{"bifrost/scroll.js", nativeScroll},
	{"bifrost/document-scroll.ts", nativeDocumentScroll},
	{"bifrost/position.js", nativePosition},
	{"bifrost/selectors.js", nativeSelectors},
	{"bifrost/data.ts", nativeData},
	{"bifrost/dom.js", nativeDOM},
	{"bifrost/events.ts", nativeEventSource},
	{"bifrost/form.tsx", nativeForm},
	{"bifrost/html.ts", nativeHTML},
	{"bifrost/intent.ts", nativeIntent},
	{"bifrost/jsx-runtime.ts", nativeJSXRuntime},
	{"bifrost/metro.js", nativeMetroHelper},
	{"bifrost/net.ts", nativeNet},
	{"bifrost/react.js", nativeReact},
	{"bifrost/refresh.ts", nativeRefresh},
	{"bifrost/navigation.tsx", nativeNavigation},
	{"bifrost/screen.tsx", nativeScreen},
	{"bifrost/text-style.js", nativeTextStyle},
	{"bifrost/place.ts", nativePlace},
	{"bifrost/text.tsx", nativeTextModule},
	{"bifrost/types.d.ts", nativeTypes},
}

// The fonts an app ships, as its overlay declares them: `<overlay>/fonts.json`
// holds one family and a file per weight. A native screen loads a font as a
// file and picks it by family, so a weight is a family of its own -- the layout
// loads every file under the name a rule looks for, and ./text-style folds the
// weight a rule asked for into that name.
type nativeFont struct {
	Family string
	Weight int
	File   string
}

func nativeFonts(overlay string) ([]nativeFont, error) {
	path := filepath.Join(overlay, "fonts.json")
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var declared []struct {
		Family string `json:"fontFamily"`
		Fonts  []struct {
			Path   string `json:"path"`
			Weight int    `json:"weight"`
		} `json:"fonts"`
	}
	if err := json.Unmarshal(content, &declared); err != nil {
		return nil, fmt.Errorf("bifrost: %s: %w", path, err)
	}
	seen := map[nativeFont]bool{}
	fonts := make([]nativeFont, 0, len(declared))
	for _, family := range declared {
		if family.Family == "" || len(family.Fonts) == 0 {
			return nil, fmt.Errorf("bifrost: %s: a family has no name or no files", path)
		}
		for _, font := range family.Fonts {
			if font.Path == "" || font.Weight <= 0 {
				return nil, fmt.Errorf("bifrost: %s: a file of %s has no path or no weight", path, family.Family)
			}
			// A file the app does not ship is a bundle that fails minutes into a
			// build; here it fails where the app can see it.
			if !nativeOverlayHas(overlay, font.Path) {
				return nil, fmt.Errorf("bifrost: %s: the overlay does not ship %s", path, font.Path)
			}
			one := nativeFont{Family: family.Family, Weight: font.Weight, File: font.Path}
			if seen[one] {
				return nil, fmt.Errorf("bifrost: %s: %s names weight %d twice", path, family.Family, font.Weight)
			}
			seen[one] = true
			fonts = append(fonts, one)
		}
	}
	sort.SliceStable(fonts, func(i, j int) bool {
		if fonts[i].Family != fonts[j].Family {
			return fonts[i].Family < fonts[j].Family
		}
		return fonts[i].Weight < fonts[j].Weight
	})
	return fonts, nil
}

// The module the layout loads: the name of each file, and the weights each
// family ships, so the text of a page finds the file for the weight a rule asked
// for. An app that declares no font gets both maps empty and renders exactly as
// it did before.
func nativeFontsModule(fonts []nativeFont) string {
	weights := map[string][]int{}
	order := []string{}
	files := make([]string, 0, len(fonts))
	for _, font := range fonts {
		if _, known := weights[font.Family]; !known {
			order = append(order, font.Family)
		}
		weights[font.Family] = append(weights[font.Family], font.Weight)
		name := strconv.Quote(font.Family + " " + strconv.Itoa(font.Weight))
		files = append(files, fmt.Sprintf("  %s: require(%s),", name, strconv.Quote("../"+filepath.ToSlash(font.File))))
	}
	var families strings.Builder
	for _, family := range order {
		named := make([]string, 0, len(weights[family]))
		for _, weight := range weights[family] {
			named = append(named, strconv.Itoa(weight))
		}
		fmt.Fprintf(&families, "  %s: [%s],\n", strconv.Quote(family), strings.Join(named, ", "))
	}
	var module strings.Builder
	module.WriteString("// The fonts the app ships, as its overlay declares them, and as the generated\n")
	module.WriteString("// layout loads them: one file per weight, under the name a rule looks for -- the\n")
	module.WriteString("// family the stylesheet names, and the weight it asked for.\n")
	module.WriteString("export const families: Record<string, number[]> = {\n")
	module.WriteString(families.String())
	module.WriteString("};\n\nexport const fonts: Record<string, number> = {\n")
	module.WriteString(strings.Join(files, "\n"))
	if len(files) > 0 {
		module.WriteString("\n")
	}
	module.WriteString("};\n")
	return module.String()
}

func nativePackageManifest() map[string]any {
	return map[string]any{
		"private": true,
		"main":    "expo-router/entry",
		"dependencies": map[string]any{
			// The calendar of the device, for the fields of a form that a browser
			// draws instead of typing into. The version is the one the runtime of
			// this SDK carries.
			"@react-native-community/datetimepicker": "9.1.0",
			// The slider of the device, for the same reason: a range is dragged,
			// not typed into. Same version of the same runtime.
			"@react-native-community/slider": "5.2.0",
			"expo":                           "~57.0.28",
			"expo-constants":                 "~57.0.21",
			"expo-document-picker":           "~57.0.3",
			"expo-font":                      "~57.0.4",
			"expo-linking":                   "~57.0.12",
			"expo-router":                    "~57.0.26",
			"expo-status-bar":                "~57.0.1",
			// The transformer of this target reads a stylesheet and rewrites its
			// selectors, which takes a parser: Metro resolves it from the project,
			// and without it every element selector is left as it was and dropped by
			// the compiler in silence.
			"postcss":                        "8.5.31",
			"postcss-selector-parser":        "7.1.6",
			"react":                          "19.2.3",
			"react-native":                   "0.86.3",
			"react-native-safe-area-context": "5.10.1",
			"react-native-screens":           "4.29.0",
			// A drawing of the web is an svg, and the target has no vector tags
			// of its own. The version is the one the runtime of this SDK carries.
			"react-native-svg": "15.15.4",
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
		// The runtime the app starts in installs a window.location of its own, and
		// the one it installs answers the url of the bundler: it is there so that a
		// relative request has an origin, and it cannot be replaced afterwards --
		// measured, the property it defines refuses both an assignment and a
		// redefinition. False is the runtime's own switch for not installing one, so
		// the address a page reads is the address of the page.
		"extra": map[string]any{"router": map[string]any{"origin": false}},
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
	same, err := writeNativeProject(native, prepared, *origin, overlay)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(os.Stdout, "Bifrost native project: %s\n", filepath.ToSlash(native))
	// A run that changed nothing says so, and that is the whole difference between a
	// run Metro hears about and one it does not: a restart of the bundler is a cold
	// bundle -- 81.6 s measured -- and there is nothing to restart for.
	if same {
		_, _ = fmt.Fprintln(os.Stdout, "  nothing changed: Metro does not have to be restarted")
	}
	if overlayExists(overlay) {
		_, _ = fmt.Fprintf(os.Stdout, "  app files from: %s\n", filepath.ToSlash(overlay))
		if warning := nativeOverlayWarning(overlay); warning != "" {
			_, _ = fmt.Fprintln(os.Stderr, warning)
		}
	}
	_, _ = fmt.Fprintf(os.Stdout, "  cd %s && bun install && bunx expo start\n", filepath.ToSlash(native))
	return nil
}

// The run says whether every file of the trees it owns already had the bytes it
// would write, which is the difference between a run Metro hears about and one it
// does not.
func writeNativeProject(native string, app appRouter, origin, overlay string) (bool, error) {
	screens := make(map[string]appRoute, len(app.Routes))
	for _, route := range app.Routes {
		view, err := expoRouterPath(route.Pattern)
		if err != nil {
			return false, err
		}
		if previous, exists := screens[view]; exists {
			return false, fmt.Errorf("bifrost: routes %s and %s both map to native path %s", previous.Pattern, route.Pattern, view)
		}
		screens[view] = route
	}
	// What this run writes under app/ and bifrost/ is recorded as it goes, so that
	// what the run before left and this one does not is taken out at the end -- and
	// a file that did not change keeps its mtime, which is what keeps Metro from
	// throwing its cache away. See writeNativeFile.
	written := newNativeWritten()
	for _, file := range nativeProjectFiles {
		// A file the overlay ships is not written here first and replaced after:
		// the app's copy is the one that stays, and writing the generated one on
		// the way is a second mtime on the same run. Measured: the app replaces
		// `metro.config.js`, and it was the only file of the project whose mtime
		// still changed on a run that changed nothing.
		if file.Name != "package.json" && nativeOverlayHas(overlay, file.Name) {
			continue
		}
		if err := written.write(native, file.Name, file.Content, 0o644); err != nil {
			return false, err
		}
	}
	// The fonts the app declares, read before the project is written: a file the
	// overlay does not ship is an error here and not a bundle that fails later.
	fonts, err := nativeFonts(overlay)
	if err != nil {
		return false, err
	}
	if err := written.write(native, "bifrost/fonts.ts", nativeFontsModule(fonts), 0o644); err != nil {
		return false, err
	}
	// The layout and the tag map are written with the overlay in hand, because
	// what they contain depends on what the app ships next to them.
	for name, content := range map[string]string{
		"app/_layout.tsx":  nativeLayout(overlay),
		"bifrost/host.tsx": nativeHost(overlay),
	} {
		if err := written.write(native, name, content, 0o644); err != nil {
			return false, err
		}
	}
	manifest := nativePackageManifest()
	if err := mergeNativeOverlayManifest(overlay, manifest); err != nil {
		return false, err
	}
	if err := written.writeJSON(native, "package.json", manifest); err != nil {
		return false, err
	}
	if err := written.writeJSON(native, "app.json", nativeAppManifest(app.ProjectRoot)); err != nil {
		return false, err
	}
	if err := written.write(native, "bifrost/origin.ts", nativeOrigin(origin), 0o644); err != nil {
		return false, err
	}
	pageViews := make([]string, 0, len(screens))
	for view, route := range screens {
		// A not-found route catches every url the app does not have, so leaving it
		// in would make every url -- an api route included -- look like a page the
		// router can paint.
		if route.NotFoundPage {
			continue
		}
		pageViews = append(pageViews, view)
	}
	if err := written.write(native, "bifrost/route.ts", nativeRoute(nativePagePaths(pageViews)), 0o644); err != nil {
		return false, err
	}
	// The intent of the system is written here and not by an app: it is the seam
	// between a url that arrived and a screen, and what it decides is the same for
	// every app -- see bifrost/intent.ts. An app that ships one of its own gets the
	// error of a file that is not a route of its tree, which is loud on purpose.
	if err := written.write(native, "app/+native-intent.tsx", nativeIntentFile, 0o644); err != nil {
		return false, err
	}
	views := filepath.Join(app.ProjectRoot, ".bifrost", "views")
	for view, route := range screens {
		source, err := screenSource(native, views, view, route, app.RouteParams[route.Pattern])
		if err != nil {
			return false, err
		}
		name := filepath.Join("app", filepath.FromSlash(view)+".tsx")
		if err := written.write(native, name, source, 0o644); err != nil {
			return false, err
		}
	}
	if err := copyNativeOverlay(native, overlay, screens, written); err != nil {
		return false, err
	}
	// Last, so that the files the overlay put under these two trees are known and
	// not taken for leftovers: an app ships `bifrost/host-extra.tsx` of its own,
	// and removing it here would put it back with a new mtime on every run, which
	// is the exact thing this is here to avoid.
	for _, tree := range []string{"app", "bifrost"} {
		if err := written.removeStrays(native, tree); err != nil {
			return false, err
		}
	}
	return written.untouched(), nil
}

// App files are copied last, so an app owns its screens, its layout and the
// client it talks to the server with, while the route table keeps owning which
// routes exist.
func copyNativeOverlay(native, overlay string, screens map[string]appRoute, written *nativeWritten) error {
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
		// The two files of the overlay the generator reads instead of copying:
		// the manifest, which is merged, and the fonts, which are written into
		// bifrost/fonts.ts.
		if name == "package.json" || name == "fonts.json" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(overlay, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		if err := written.write(native, name, string(content), 0o644); err != nil {
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

// The pages of an app in the shape a url spells them, so that a page telling
// where it is can be told apart from anything else it might navigate to.
func nativePagePaths(views []string) []string {
	paths := make([]string, 0, len(views))
	for _, view := range views {
		path := "/" + view
		path = strings.TrimSuffix(path, "/index")
		if path == "" {
			path = "/"
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// Where the page on screen is. A browser keeps an address and a page reads it
// whenever it likes; here the address is a value of the navigation state, and
// the only component that can read it is the screen. So the screen on top writes
// it down while it renders, and the browser shim hands it back as
// window.location. The pages of the app travel with it, because a url that is
// not one of them is not a page this router can paint.
func nativeRoute(pages []string) string {
	var list strings.Builder
	for index, page := range pages {
		if index > 0 {
			list.WriteString(",\n  ")
		}
		list.WriteString(strconv.Quote(page))
	}
	return nativeRouteHead + "\n  " + list.String() + "\n]" + nativeRouteRest
}

const nativeRouteHead = `// Where the page on screen is. A browser keeps an address and a page reads it
// whenever it likes; here the address is a value of the navigation state, and the
// only component that can read it is the screen. So the screen on top writes it
// down as it renders, before anything inside it renders -- a page reads the
// address in its first effect, and the effect of a child runs before the effect
// of its parent -- and the browser shim hands it back as window.location.

let address = "/";
const listeners = new Set<() => void>();

export function setAddress(next: string): void {
  if (next === address) {
    return;
  }
  address = next;
  for (const listener of listeners) {
    listener();
  }
}

export function getAddress(): string {
  return address;
}

// A change the shim did not ask for is a move of the router -- the back button,
// a gesture, a link -- and it is the one a page hears as a popstate.
export function onAddressChange(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

// The pages of this app, in the shape the router spells them: a segment in
// brackets is one the router fills in, and a segment that starts with three dots
// takes the rest of the url. What is not here is not a page of the app, and the
// web would have asked the server for it and landed on whatever it answered. A
// not-found route is left out: it catches every url the app does not have, so it
// would make every url look like a page.
const PAGES = [`

const nativeRouteRest = `

function matched(page: string, wanted: string[]): boolean {
  const parts = page.split("/").filter((part) => part !== "");
  for (let index = 0; index < parts.length; index++) {
    const part = parts[index];
    if (part.indexOf("[...") === 0) {
      return wanted.length >= index;
    }
    if (index >= wanted.length) {
      return false;
    }
    if (part.charAt(0) === "[" && part.charAt(part.length - 1) === "]") {
      continue;
    }
    if (part !== wanted[index]) {
      return false;
    }
  }
  return wanted.length === parts.length;
}

export function hasPage(url: string): boolean {
  const path = url.split("?")[0].split("#")[0];
  const wanted = path.split("/").filter((part) => part !== "");
  return PAGES.some((page) => matched(page, wanted));
}
`

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

// A file is written only when what is on disk is not already what goes there.
//
// The generated project is a build artifact and every run rewrites it, so it looks
// like the mtime does not matter -- and it does, because Metro watches this tree:
// a file that is replaced with the same bytes is a change for the watcher, and the
// bundler throws its cache away for it. Measured: `bifrost native` with nothing
// changed cost a cold bundle of 81.6 s, and the only reason the run could not be
// free is that every file of the project was being replaced. An app that changes
// one screen pays for one screen.
func writeNativeFile(native, name, content string, mode os.FileMode) error {
	_, err := writeNativeFileTelling(native, name, content, mode)
	return err
}

// The same, saying whether the bytes on disk had to change. A run that changes
// nothing is a run Metro does not hear about, which is the whole point: see
// writeNativeFile.
func writeNativeFileTelling(native, name, content string, mode os.FileMode) (bool, error) {
	filePath := filepath.Join(native, filepath.FromSlash(name))
	if current, err := os.ReadFile(filePath); err == nil && string(current) == content {
		if info, err := os.Stat(filePath); err == nil && info.Mode().Perm() == mode.Perm() {
			return false, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(filePath, []byte(content), mode)
}

// What a run wrote, so that what the run before wrote and this one did not can be
// taken out. The trees of the project used to be deleted whole before writing
// them again, which is right about what to keep and wrong about the mtime: see
// writeNativeFile.
type nativeWritten struct {
	kept    map[string]bool
	changed bool
}

func newNativeWritten() *nativeWritten {
	return &nativeWritten{kept: map[string]bool{}}
}

func (w *nativeWritten) add(name string) string {
	w.kept[filepath.ToSlash(filepath.Clean(name))] = true
	return name
}

// A file this run wrote, because what was on disk was not what goes there.
func (w *nativeWritten) wrote() {
	w.changed = true
}

// Write a file of the generated project: it is recorded as written -- so a later
// run knows it is not a leftover -- and the run learns whether the bytes on disk
// had to change. Every write of the trees the generator owns goes through here,
// and a write that goes around it is a run that says nothing changed when it did.
func (w *nativeWritten) write(native, name, content string, mode os.FileMode) error {
	w.add(name)
	changed, err := writeNativeFileTelling(native, name, content, mode)
	if err != nil {
		return err
	}
	if changed {
		w.wrote()
	}
	return nil
}

func (w *nativeWritten) writeJSON(native, name string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return w.write(native, name, string(content)+"\n", 0o644)
}

// The bytes of every file of the trees the generator owns are the ones it would
// write: the run can say it and Metro never hears about it.
func (w *nativeWritten) untouched() bool {
	return !w.changed
}

// A file of the tree that this run did not write is a leftover of the run before:
// a route the app deleted, a module it renamed. The tree is a build artifact, so
// nothing in it is worth keeping.
func (w *nativeWritten) removeStrays(native, tree string) error {
	root := filepath.Join(native, tree)
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(native, path)
		if err != nil {
			return err
		}
		if w.kept[filepath.ToSlash(relative)] {
			return nil
		}
		w.wrote()
		return os.Remove(path)
	})
}

func writeNativeJSON(native, name string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeNativeFile(native, name, string(content)+"\n", 0o644)
}
