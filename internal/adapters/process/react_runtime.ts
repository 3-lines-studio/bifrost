import nodeFs from "fs";
import nodePath from "path";

const socket = process.env.BIFROST_SOCKET;
const isDev =
  process.env.BIFROST_DEV === "1" || process.env.BIFROST_DEV === "true";

const tailwindPlugin: Bun.BunPlugin | undefined = BIFROST_TAILWIND_PLUGIN;
const reactCompilerPlugin: Bun.BunPlugin | undefined = BIFROST_REACT_COMPILER_PLUGIN;
interface ErrorDetail {
  message: string;
  position?: {
    file?: string;
    line: number;
    column: number;
    lineText?: string;
  };
  specifier?: string;
  referrer?: string;
}

interface BuildEntryResult {
  script: string;
  criticalCSS: string;
  css: string;
  cssFiles?: string[];
  chunks: string[];
}

interface RenderResult {
  html?: string;
  head?: string;
}

function serializeError(error: unknown): {
  message: string;
  stack?: string;
  position?: {
    file?: string;
    line: number;
    column: number;
    lineText?: string;
  };
  specifier?: string;
  referrer?: string;
} {
  if (error instanceof Error) {
    const result: any = { message: error.message, stack: error.stack };
    if ((error as any).position) result.position = (error as any).position;
    if ((error as any).specifier) result.specifier = (error as any).specifier;
    if ((error as any).referrer) result.referrer = (error as any).referrer;
    return result;
  }
  return { message: String(error) };
}

const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

function be32(n: number): Uint8Array {
  const b = new Uint8Array(4);
  new DataView(b.buffer).setUint32(0, n, false);
  return b;
}

function writeFrame(socket: any, kind: number, parts: Uint8Array[]): void {
  let total = 0;
  for (const part of parts) {
    total += part.length;
  }
  const header = new Uint8Array(5);
  const dv = new DataView(header.buffer);
  dv.setUint8(0, kind);
  dv.setUint32(1, total, false);
  socket.write(header);
  for (const part of parts) {
    socket.write(part);
  }
}

function writeJsonFrame(socket: any, kind: number, value: unknown): void {
  writeFrame(socket, kind, [textEncoder.encode(JSON.stringify(value))]);
}

function writeRenderFrame(socket: any, head: string, html: string): void {
  const h = textEncoder.encode(head);
  const b = textEncoder.encode(html);
  socket.write(new Uint8Array([0]));
  socket.write(be32(h.length));
  socket.write(h);
  socket.write(be32(b.length));
  socket.write(b);
}

function errorPayload(
  message: string,
  err?: { errors?: ErrorDetail[] } | Error,
): { message: string; stack?: string; errors?: ErrorDetail[] } {
  const result: { message: string; stack?: string; errors?: ErrorDetail[] } = {
    message,
  };

  if (err) {
    if ("errors" in err && Array.isArray(err.errors)) {
      result.errors = err.errors;
    } else if (err instanceof Error) {
      const serialized = serializeError(err);
      result.stack = serialized.stack;
      if (serialized.position || serialized.specifier || serialized.referrer) {
        result.errors = [{
          message: serialized.message,
          position: serialized.position,
          specifier: serialized.specifier,
          referrer: serialized.referrer,
        }];
      }
    }
  }

  return result;
}

function writeErrorFrame(
  socket: any,
  message: string,
  err?: { errors?: ErrorDetail[] } | Error,
): void {
  writeJsonFrame(socket, 1, errorPayload(message, err));
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

function allCssHrefsFromBuildOutputs(
  buildResult: Awaited<ReturnType<typeof Bun.build>>,
): string[] {
  const hrefs = buildResult.outputs
    .filter((o) => o.path.endsWith(".css"))
    .map((o) => "/dist/" + nodePath.basename(o.path));
  return [...new Set(hrefs)].sort();
}

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

function buildEntriesPayload(
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

const componentCache = new Map<
  string,
  { Component: any; Head?: any }
>();

async function handleRender(socket: any, body: {
  path?: string;
  props?: Record<string, unknown>;
}): Promise<void> {
  const { path, props } = body;

  if (!path) {
    return writeErrorFrame(socket, "Missing 'path' in request");
  }

  const importPath = isDev
    ? `${path}?t=${Date.now()}`
    : path.startsWith("/")
      ? "file://" + path
      : path;

  try {
    const mod = await import(importPath);

    if (typeof mod.render === "function") {
      const result: RenderResult = await mod.render(props || {});
      return writeRenderFrame(socket, result.head ?? "", result.html ?? "");
    }

    const cached = componentCache.get(path);
    let Component: any;
    let Head: any | undefined;

    if (!isDev && cached) {
      Component = cached.Component;
      Head = cached.Head;
    } else {
      Component =
        mod.default ||
        mod.Page ||
        Object.values(mod).find((x: any) => typeof x === "function");
      Head = mod.Head;

      if (!isDev && Component) {
        componentCache.set(path, { Component, Head });
      }
    }

    if (!Component) {
      return writeErrorFrame(
        socket,
        `No component export found in ${path}. Expected default export, Page export, or a function export.`,
      );
    }

    const React = await import("react");
    const { renderToString } = await import("react-dom/server");

    const componentProps = props || {};

    let head = "";
    if (Head) {
      try {
        const headEl = React.createElement(Head, componentProps);
        head = renderToString(headEl);
      } catch (headErr) {
        console.error("Error rendering head:", headErr);
      }
    }

    const el = React.createElement(Component, componentProps);

    let html: string;
    try {
      html = renderToString(el);
    } catch (renderErr) {
      const message =
        renderErr instanceof Error ? renderErr.message : String(renderErr);
      return writeErrorFrame(socket, `Render error: ${message}`, renderErr);
    }

    return writeRenderFrame(socket, head, html);
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    return writeErrorFrame(socket, `Failed to import component: ${message}`, err);
  }
}

async function handleBuild(socket: any, body: {
  entrypoints?: string[];
  outdir?: string;
  target?: string;
  entryNames?: string[];
  framework?: string;
}): Promise<void> {
  const { entrypoints, outdir, target, entryNames } = body;

  if (!Array.isArray(entrypoints) || entrypoints.length === 0) {
    return writeJsonFrame(socket, 2, {
      ok: false,
      error: errorPayload("Missing entrypoints"),
    });
  }

  if (!outdir) {
    return writeJsonFrame(socket, 2, {
      ok: false,
      error: errorPayload("Missing outdir"),
    });
  }

  const buildTarget = target === "bun" ? "bun" : "browser";
  const isSSR = buildTarget === "bun";
  const hashClientAssets =
    (process.env.BIFROST_PROD === "1" ||
      process.env.BIFROST_PROD === "true") &&
    !isSSR;

  try {
    const plugins: any[] = [
      ...(reactCompilerPlugin ? [reactCompilerPlugin] : []),
      ...(!isSSR && tailwindPlugin ? [tailwindPlugin] : []),
    ];

    const naming = hashClientAssets
      ? {
          entry: "[name]-[hash].[ext]",
          chunk: "[name]-[hash].[ext]",
          asset: "[name]-[hash].[ext]",
        }
      : entryNames && entryNames.length > 0
        ? { entry: "[name].[ext]" }
        : undefined;

    const result = await Bun.build({
      entrypoints,
      outdir,
      target: buildTarget,
      minify: !isDev,
      splitting: !isSSR,
      naming,
      plugins,
      metafile: true,
      ...(!isDev
        ? { define: { "process.env.NODE_ENV": '"production"' } }
        : {}),
    });

    if (!result.success) {
      const errors = result.logs
        .filter((log) => log.level === "error")
        .map((log) => ({
          message: log.message,
          position: log.position
            ? {
                file: log.file,
                line: log.position.line,
                column: log.position.column,
                lineText: log.position.lineText,
              }
            : undefined,
          specifier: log.data?.specifier,
          referrer: log.data?.referrer,
        }));

      return writeJsonFrame(socket, 2, {
        ok: false,
        error: errorPayload("Build failed", { errors }),
      });
    }

    if (!hashClientAssets && entryNames && entryNames.length === entrypoints.length) {
      for (let i = 0; i < entrypoints.length; i++) {
        const entryPath = entrypoints[i];
        const entryName = entryNames[i];
        const ext = nodePath.extname(entryPath);
        const oldName = nodePath.basename(entryPath, ext) + ".js";
        const newName = entryName + ".js";
        if (oldName !== newName) {
          const oldPath = nodePath.join(outdir, oldName);
          const newPath = nodePath.join(outdir, newName);
          try {
            nodeFs.renameSync(oldPath, newPath);
          } catch {}
        }
        const oldCssName = nodePath.basename(entryPath, ext) + ".css";
        const newCssName = entryName + ".css";
        if (oldCssName !== newCssName) {
          const oldCssPath = nodePath.join(outdir, oldCssName);
          const newCssPath = nodePath.join(outdir, newCssName);
          try {
            nodeFs.renameSync(oldCssPath, newCssPath);
          } catch {}
        }
      }
    }

    let entries: Record<string, BuildEntryResult>;
    try {
      entries = buildEntriesPayload(
        result,
        entrypoints,
        entryNames ?? [],
        hashClientAssets,
        outdir,
      );
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      return writeJsonFrame(socket, 2, {
        ok: false,
        error: errorPayload(`Build output mapping failed: ${message}`, err as Error),
      });
    }

    return writeJsonFrame(socket, 2, { ok: true, entries });
  } catch (err) {
    return writeJsonFrame(socket, 2, {
      ok: false,
      error: errorPayload("Build failed", err as Error),
    });
  }
}

interface SocketState {
  buf: Uint8Array;
}

function concatBytes(a: Uint8Array, b: Uint8Array): Uint8Array {
  const out = new Uint8Array(a.length + b.length);
  out.set(a);
  out.set(b);
  return out;
}

Bun.listen({
  unix: socket,
  socket: {
    open(sock: any) {
      sock.data = { buf: new Uint8Array(0) } as SocketState;
    },
    data(sock: any, data: Uint8Array) {
      const state: SocketState = sock.data;
      let buf = state.buf.length > 0 ? concatBytes(state.buf, data) : data;
      state.buf = new Uint8Array(0);
      while (buf.length >= 5) {
        const kind = buf[0];
        const len = new DataView(buf.buffer, buf.byteOffset + 1, 4).getUint32(0, false);
        if (buf.length < 5 + len) {
          break;
        }
        const payload = buf.slice(5, 5 + len);
        buf = buf.slice(5 + len);
        void dispatchFrame(sock, kind, payload);
      }
      state.buf = buf;
    },
    close() {},
    error(sock: any, error: unknown) {
      console.error("bifrost runtime socket error:", error);
    },
    idleTimeout: 0,
  },
});

async function dispatchFrame(sock: any, kind: number, payload: Uint8Array): Promise<void> {
  let body: any;
  try {
    body = JSON.parse(textDecoder.decode(payload));
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    return writeErrorFrame(sock, `Failed to parse request: ${message}`);
  }
  try {
    if (kind === 2) {
      await handleBuild(sock, body);
    } else {
      await handleRender(sock, body);
    }
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    writeErrorFrame(sock, message, err);
  }
}
