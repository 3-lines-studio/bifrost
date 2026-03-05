import nodeFs from "fs";
import nodePath from "path";

const socket = process.env.BIFROST_SOCKET;
const isDev =
  process.env.BIFROST_DEV === "1" || process.env.BIFROST_DEV === "true";

const sveltePlugin = (await import("bun-plugin-svelte")).default;
const tailwindPlugin = (await import("bun-plugin-tailwind")).default;

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

interface Result {
  ok?: boolean;
  error?: {
    message: string;
    stack?: string;
    errors?: ErrorDetail[];
  };
}

interface RenderResult {
  html: string;
  head?: string;
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
  const result: Result = {
    error: {
      message,
    },
  };

  if (err) {
    if ("errors" in err && Array.isArray(err.errors)) {
      result.error!.errors = err.errors;
    } else if (err instanceof Error) {
      const serialized = serializeError(err);
      result.error!.stack = serialized.stack;
    }
  }

  return new Response(JSON.stringify(result) + "\n");
}

const componentCache = new Map<
  string,
  { Component: any }
>();

async function handleRender(req: Bun.BunRequest): Promise<Response> {
  let body: { path?: string; props?: Record<string, unknown> };
  try {
    body = await req.json();
  } catch (err) {
    const message = err instanceof Error ? err.message : "Invalid JSON body";
    return createError(`Failed to parse request: ${message}`);
  }

  const { path, props } = body;

  if (!path) {
    return createError("Missing 'path' in request");
  }

  const importPath = isDev ? `${path}?t=${Date.now()}` : path;

  try {
    const mod = await import(importPath);

    if (typeof mod.render === "function") {
      const result: RenderResult = await mod.render(props || {});
      return new Response(JSON.stringify(result) + "\n");
    }

    const cached = componentCache.get(path);
    let Component: any;

    if (!isDev && cached) {
      Component = cached.Component;
    } else {
      Component = mod.default || Object.values(mod).find((x: any) => typeof x === "function");

      if (!isDev && Component) {
        componentCache.set(path, { Component });
      }
    }

    if (!Component) {
      return createError(
        `No component export found in ${path}. Expected default export or a function export.`,
      );
    }

    const { render } = await import("svelte/server");

    const componentProps = props || {};

    let result;
    try {
      result = render(Component, { props: componentProps });
    } catch (renderErr) {
      const message =
        renderErr instanceof Error ? renderErr.message : String(renderErr);
      return createError(`Render error: ${message}`, renderErr);
    }

    const output: RenderResult = { 
      html: result.body, 
      head: result.head || ""
    };
    return new Response(JSON.stringify(output) + "\n");
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
    return createError(`Failed to parse request: ${message}`, err);
  }

  const { entrypoints, outdir, target, entryNames } = body;

  if (!Array.isArray(entrypoints) || entrypoints.length === 0) {
    return createError("Missing entrypoints");
  }

  if (!outdir) {
    return createError("Missing outdir");
  }

  const isProduction =
    process.env.BIFROST_PROD === "1" || process.env.BIFROST_PROD === "true";

  const buildTarget = target === "bun" ? "bun" : "browser";
  const isSSR = buildTarget === "bun";

  try {
    const plugins = isSSR ? [sveltePlugin] : [sveltePlugin, tailwindPlugin];

    const naming = isProduction
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
    });

    if (entryNames && entryNames.length === entrypoints.length) {
      for (let i = 0; i < entrypoints.length; i++) {
        const entryPath = entrypoints[i];
        const entryName = entryNames[i];
        const ext = nodePath.extname(entryPath);
        const oldName = nodePath.basename(entryPath, ext) + ".js";
        const newName = entryName + ".js";
        if (oldName !== newName) {
          const oldPath = nodePath.join(outdir, oldName);
          const newPath = nodePath.join(outdir, newName);
          try { nodeFs.renameSync(oldPath, newPath); } catch {}
        }
        const oldCssName = nodePath.basename(entryPath, ext) + ".css";
        const newCssName = entryName + ".css";
        if (oldCssName !== newCssName) {
          const oldCssPath = nodePath.join(outdir, oldCssName);
          const newCssPath = nodePath.join(outdir, newCssName);
          try { nodeFs.renameSync(oldCssPath, newCssPath); } catch {}
        }
      }
    }

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

    const response: Result = { ok: true };
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
