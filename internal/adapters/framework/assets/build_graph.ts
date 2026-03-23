// Canonical Bun build graph helpers. Also inlined in react_dev.ts (stdin has no stable import root).
// Keep both copies in sync when changing build output mapping.
import nodePath from "path";

export interface BuildEntryResult {
  script: string;
  criticalCSS: string;
  css: string;
  cssFiles?: string[];
  chunks: string[];
}

function entryStemMatchesJs(base: string, stem: string): boolean {
  return (
    base === `${stem}.js` ||
    (base.startsWith(`${stem}-`) && base.endsWith(".js"))
  );
}

function entryStemMatchesCss(base: string, stem: string): boolean {
  return (
    base === `${stem}.css` ||
    (base.startsWith(`${stem}-`) && base.endsWith(".css"))
  );
}

function collectChunkURLs(
  outputs: Awaited<ReturnType<typeof Bun.build>>["outputs"],
): string[] {
  return outputs
    .filter((o) => o.kind === "chunk" && o.path.endsWith(".js"))
    .map((o) => "/dist/" + nodePath.basename(o.path))
    .sort();
}

function resolveMetaOutputKey(
  metaOutputs: NonNullable<Bun.BuildMetafile["outputs"]>,
  filePath: string,
): string | undefined {
  const want = nodePath.resolve(filePath);
  for (const k of Object.keys(metaOutputs)) {
    if (nodePath.resolve(k) === want) return k;
  }
  const base = nodePath.basename(filePath);
  for (const k of Object.keys(metaOutputs)) {
    if (nodePath.basename(k) === base) return k;
  }
  return undefined;
}

function artifactForChunkImport(
  buildResult: Awaited<ReturnType<typeof Bun.build>>,
  impPath: string,
): (typeof buildResult.outputs)[number] | undefined {
  const resolvedImp = nodePath.resolve(impPath);
  let art = buildResult.outputs.find(
    (o) => nodePath.resolve(o.path) === resolvedImp,
  );
  if (art) return art;
  const base = nodePath.basename(impPath);
  return buildResult.outputs.find(
    (o) => o.kind === "chunk" && nodePath.basename(o.path) === base,
  );
}

function artifactForImport(
  buildResult: Awaited<ReturnType<typeof Bun.build>>,
  impPath: string,
): (typeof buildResult.outputs)[number] | undefined {
  const resolvedImp = nodePath.resolve(impPath);
  let art = buildResult.outputs.find(
    (o) => nodePath.resolve(o.path) === resolvedImp,
  );
  if (art) return art;
  const base = nodePath.basename(impPath);
  return buildResult.outputs.find(
    (o) => nodePath.basename(o.path) === base,
  );
}

function dedupeOrderedStylesheetHrefs(urls: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const u of urls) {
    if (!u || seen.has(u)) continue;
    seen.add(u);
    out.push(u);
  }
  return out;
}

/** All stylesheet URLs from build outputs (multi-entry shared Tailwind often emits one CSS file not linked in every entry's meta graph). */
function allCssHrefsFromBuildOutputs(
  buildResult: Awaited<ReturnType<typeof Bun.build>>,
): string[] {
  const hrefs = buildResult.outputs
    .filter((o) => o.path.endsWith(".css"))
    .map((o) => "/dist/" + nodePath.basename(o.path));
  return [...new Set(hrefs)].sort();
}

/** CSS outputs reachable from this entry via the module graph (shared bundles under code splitting). */
function collectCssForEntry(
  buildResult: Awaited<ReturnType<typeof Bun.build>>,
  entryJsAbsPath: string,
): string[] {
  const meta = buildResult.metafile;
  if (!meta?.outputs) {
    return [];
  }
  const metaOutputs = meta.outputs;
  const startKey = resolveMetaOutputKey(metaOutputs, entryJsAbsPath);
  if (!startKey) {
    return [];
  }

  const seenMeta = new Set<string>();
  const hrefs: string[] = [];

  function visit(metaKey: string) {
    if (seenMeta.has(metaKey)) return;
    seenMeta.add(metaKey);
    const node = metaOutputs[metaKey];
    if (!node?.imports) return;
    for (const imp of node.imports) {
      const impPath = imp.path;
      if (impPath.endsWith(".css")) {
        const art = artifactForImport(buildResult, impPath);
        if (art?.path.endsWith(".css")) {
          hrefs.push("/dist/" + nodePath.basename(art.path));
        }
        continue;
      }
      if (!impPath.endsWith(".js")) continue;
      const art = artifactForChunkImport(buildResult, impPath);
      if (!art || art.kind !== "chunk") continue;
      const childKey = resolveMetaOutputKey(metaOutputs, art.path);
      if (childKey) visit(childKey);
    }
  }

  visit(startKey);
  return [...new Set(hrefs)].sort();
}

/** Chunks reachable from this entry only (correct for multi-entry shared vendor builds). */
function collectChunksForEntry(
  buildResult: Awaited<ReturnType<typeof Bun.build>>,
  entryJsAbsPath: string,
): string[] {
  const meta = buildResult.metafile;
  if (!meta?.outputs) {
    return collectChunkURLs(buildResult.outputs);
  }
  const metaOutputs = meta.outputs;
  const startKey = resolveMetaOutputKey(metaOutputs, entryJsAbsPath);
  if (!startKey) {
    return collectChunkURLs(buildResult.outputs);
  }

  const seen = new Set<string>();
  const hrefs: string[] = [];

  function visit(metaKey: string) {
    if (seen.has(metaKey)) return;
    seen.add(metaKey);
    const node = metaOutputs[metaKey];
    if (!node?.imports) return;
    for (const imp of node.imports) {
      const impPath = imp.path;
      if (!impPath.endsWith(".js")) continue;
      const art = artifactForChunkImport(buildResult, impPath);
      if (!art || art.kind !== "chunk") continue;
      hrefs.push("/dist/" + nodePath.basename(art.path));
      const childKey = resolveMetaOutputKey(metaOutputs, art.path);
      if (childKey) visit(childKey);
    }
  }

  visit(startKey);
  return [...new Set(hrefs)].sort();
}

export function buildEntriesPayload(
  buildResult: Awaited<ReturnType<typeof Bun.build>>,
  entrypoints: string[],
  entryNames: string[],
  isProduction: boolean,
  outdir: string,
): Record<string, BuildEntryResult> {
  if (!entryNames || entryNames.length !== entrypoints.length) {
    return {};
  }
  const out: Record<string, BuildEntryResult> = {};
  for (let i = 0; i < entrypoints.length; i++) {
    const entryName = entryNames[i]!;
    const stem = nodePath.basename(
      entrypoints[i]!,
      nodePath.extname(entrypoints[i]!),
    );
    let script: string;
    let css: string;
    let entryAbs: string;
    if (isProduction) {
      const ep = buildResult.outputs.find(
        (o) =>
          o.kind === "entry-point" &&
          o.path.endsWith(".js") &&
          entryStemMatchesJs(nodePath.basename(o.path), stem),
      );
      if (!ep) {
        throw new Error(`No entry-point .js output for entry stem "${stem}"`);
      }
      script = "/dist/" + nodePath.basename(ep.path);
      entryAbs = nodePath.resolve(ep.path);
      const cssArt = buildResult.outputs.find(
        (o) =>
          o.path.endsWith(".css") &&
          entryStemMatchesCss(nodePath.basename(o.path), stem),
      );
      const stemCss = cssArt ? "/dist/" + nodePath.basename(cssArt.path) : "";
      const graphCss = collectCssForEntry(buildResult, entryAbs);
      let ordered = dedupeOrderedStylesheetHrefs(
        stemCss ? [stemCss, ...graphCss] : [...graphCss],
      );
      if (ordered.length === 0) {
        ordered = allCssHrefsFromBuildOutputs(buildResult);
      }
      css = ordered[0] ?? "";
      const cssFiles = ordered.slice(1);
      const chunks = collectChunksForEntry(buildResult, entryAbs);
      out[entryName] = {
        script,
        criticalCSS: "",
        css,
        ...(cssFiles.length > 0 ? { cssFiles } : {}),
        chunks,
      };
    } else {
      script = "/dist/" + entryName + ".js";
      css = "/dist/" + entryName + ".css";
      entryAbs = nodePath.resolve(nodePath.join(outdir, entryName + ".js"));
      const chunks = collectChunksForEntry(buildResult, entryAbs);
      out[entryName] = { script, criticalCSS: "", css, chunks };
    }
  }
  return out;
}
