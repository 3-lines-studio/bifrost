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
  stream?: ReadableStream<Uint8Array>;
}

function serializeError(error: unknown): {
  message: string;
  stack?: string;
} {
  if (error instanceof Error) {
    return {
      message: error.message,
      stack: error.stack,
    };
  }
  return { message: String(error) };
}

function createError(
  message: string,
  err?: { errors?: ErrorDetail[] } | Error,
): Response {
  const result: {
    error: {
      message: string;
      stack?: string;
      errors?: ErrorDetail[];
    };
  } = {
    error: {
      message,
    },
  };

  if (err) {
    if ("errors" in err && Array.isArray(err.errors)) {
      result.error.errors = err.errors;
    } else if (err instanceof Error) {
      const serialized = serializeError(err);
      result.error.stack = serialized.stack;
    }
  }

  return new Response(JSON.stringify(result) + "\n");
}

function singleLineRenderResponse(head: string, html: string): Response {
  return new Response(JSON.stringify({ head, html }) + "\n", {
    headers: {
      "Content-Type": "application/x-ndjson; charset=utf-8",
    },
  });
}

function ndjsonRenderResponse(head: string, html: string): Response {
  const enc = new TextEncoder();
  const line1 = JSON.stringify({ head }) + "\n";
  const line2 = JSON.stringify({ html }) + "\n";
  return new Response(
    new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(enc.encode(line1));
        controller.enqueue(enc.encode(line2));
        controller.close();
      },
    }),
    {
      headers: {
        "Content-Type": "application/x-ndjson; charset=utf-8",
      },
    },
  );
}

function headThenRawStreamResponse(
  head: string,
  htmlStream: ReadableStream<Uint8Array>,
): Response {
  const enc = new TextEncoder();
  const headLine = enc.encode(JSON.stringify({ head }) + "\n");
  return new Response(
    new ReadableStream<Uint8Array>({
      async start(controller) {
        controller.enqueue(headLine);
        const reader = htmlStream.getReader();
        try {
          while (true) {
            const { done, value } = await reader.read();
            if (done) break;
            if (value) controller.enqueue(value);
          }
        } finally {
          reader.releaseLock();
        }
        controller.close();
      },
    }),
    {
      headers: {
        "Content-Type": "application/x-ndjson; charset=utf-8",
      },
    },
  );
}

function renderResponse(head: string, html: string): Response {
  if (isDev) {
    return ndjsonRenderResponse(head, html);
  }
  return singleLineRenderResponse(head, html);
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

async function handleRender(req: Bun.BunRequest): Promise<Response> {
  let body: {
    path?: string;
    props?: Record<string, unknown>;
    streamBody?: boolean;
  };
  try {
    body = await req.json();
  } catch (err) {
    const message = err instanceof Error ? err.message : "Invalid JSON body";
    return createError(`Failed to parse request: ${message}`);
  }

  const { path, props, streamBody } = body;
  const wantStream = streamBody === true;

  if (!path) {
    return createError("Missing 'path' in request");
  }

  const importPath = isDev
    ? `${path}?t=${Date.now()}`
    : path.startsWith("/")
      ? "file://" + path
      : path;

  try {
    const mod = await import(importPath);

    if (typeof mod.render === "function") {
      const result: RenderResult = await mod.render(props || {}, {
        streamBody: wantStream,
      });
      if (result.stream instanceof ReadableStream) {
        return headThenRawStreamResponse(result.head ?? "", result.stream);
      }
      return renderResponse(result.head ?? "", result.html ?? "");
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
      return createError(
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

    if (wantStream) {
      try {
        const { renderToReadableStream } = await import("react-dom/server");
        const stream = await renderToReadableStream(el);
        return headThenRawStreamResponse(head, stream);
      } catch {
        let html: string;
        try {
          html = renderToString(el);
        } catch (renderErr) {
          const message =
            renderErr instanceof Error ? renderErr.message : String(renderErr);
          return createError(`Render error: ${message}`, renderErr);
        }
        return renderResponse(head, html);
      }
    }

    let html: string;
    try {
      html = renderToString(el);
    } catch (renderErr) {
      const message =
        renderErr instanceof Error ? renderErr.message : String(renderErr);
      return createError(`Render error: ${message}`, renderErr);
    }

    return renderResponse(head, html);
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    return createError(`Failed to import component: ${message}`, err);
  }
}

async function handleBuild(req: Bun.BunRequest): Promise<Response> {
  let body: {
    entrypoints?: string[];
    outdir?: string;
    target?: string;
    entryNames?: string[];
  };
  try {
    body = await req.json();
  } catch (err) {
    const message = err instanceof Error ? err.message : "Invalid JSON body";
    return createError(`Failed to parse request: ${message}`);
  }

  const { entrypoints, outdir, target, entryNames } = body;

  if (!Array.isArray(entrypoints) || entrypoints.length === 0) {
    return createError("Missing entrypoints");
  }

  if (!outdir) {
    return createError("Missing outdir");
  }

  const buildTarget = target === "bun" ? "bun" : "browser";
  const isSSR = buildTarget === "bun";
  const hashClientAssets =
    (process.env.BIFROST_PROD === "1" ||
      process.env.BIFROST_PROD === "true") &&
    !isSSR;

  try {
    const plugins = [
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

      return createError("Build failed", { errors });
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
      return createError(`Build output mapping failed: ${message}`, err as Error);
    }

    return new Response(JSON.stringify({ ok: true, entries }) + "\n");
  } catch (err) {
    return createError("Build failed", err as Error);
  }
}

Bun.serve({
  unix: socket,
  routes: {
    "/render": { POST: handleRender },
    "/build": { POST: handleBuild },
  },
});
