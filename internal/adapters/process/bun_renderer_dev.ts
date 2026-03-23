import nodeFs from "fs";
import nodePath from "path";

import { buildEntriesPayload, type BuildEntryResult } from "../framework/assets/build_graph";
import {
  createError,
  headThenRawStreamResponse,
  type ErrorDetail,
} from "../framework/assets/render_protocol";

const socket = process.env.BIFROST_SOCKET;
const isDev =
  process.env.BIFROST_DEV === "1" || process.env.BIFROST_DEV === "true";

const tailwindPlugin = (await import("bun-plugin-tailwind")).default;
const reactCompiler = (await import("../framework/assets/react_compiler_plugin")).default;

interface Result {
  ok?: boolean;
  entries?: Record<string, BuildEntryResult>;
  error?: {
    message: string;
    stack?: string;
    errors?: ErrorDetail[];
  };
}

interface RenderResult {
  html?: string;
  head?: string;
  stream?: ReadableStream<Uint8Array>;
}

/** Two NDJSON lines: head line then body line (enables Go to flush HTML shell early). */
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

  const importPath = isDev ? `${path}?t=${Date.now()}` : path;

  try {
    const mod = await import(importPath);

    if (typeof mod.render === "function") {
      const result: RenderResult = await mod.render(props || {}, {
        streamBody: wantStream,
      });
      if (result.stream instanceof ReadableStream) {
        return headThenRawStreamResponse(result.head ?? "", result.stream);
      }
      return ndjsonRenderResponse(
        result.head ?? "",
        result.html ?? "",
      );
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
      } catch (streamErr) {
        let html: string;
        try {
          html = renderToString(el);
        } catch (renderErr) {
          const message =
            renderErr instanceof Error ? renderErr.message : String(renderErr);
          return createError(`Render error: ${message}`, renderErr);
        }
        return new Response(JSON.stringify({ head, html }) + "\n", {
          headers: { "Content-Type": "application/x-ndjson; charset=utf-8" },
        });
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

    return ndjsonRenderResponse(head, html);
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    return createError(`Failed to import component: ${message}`, err);
  }
}

async function handleBuild(req: Bun.BunRequest): Promise<Response> {
  let body: { entrypoints?: string[]; outdir?: string; target?: string; entryNames?: string[] };
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
    const plugins = isSSR ? [reactCompiler] : [reactCompiler, tailwindPlugin];

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

    const response: Result = { ok: true, entries };
    return new Response(JSON.stringify(response) + "\n");
  } catch (err) {
    return createError("Build failed", err);
  }
}

Bun.serve({
  unix: socket,
  routes: {
    "/render": { POST: handleRender },
    "/build": { POST: handleBuild },
  },
});
