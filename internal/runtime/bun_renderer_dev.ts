const socket = process.env.BIFROST_SOCKET;
const isDev =
  process.env.BIFROST_DEV === "1" || process.env.BIFROST_DEV === "true";

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
  { Component: any; Head?: any }
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

    let html: string;
    try {
      const el = React.createElement(Component, componentProps);
      html = renderToString(el);
    } catch (renderErr) {
      const message =
        renderErr instanceof Error ? renderErr.message : String(renderErr);
      return createError(`Render error: ${message}`, renderErr);
    }

    let head = "";
    if (Head) {
      try {
        const headEl = React.createElement(Head, componentProps);
        head = renderToString(headEl);
      } catch (headErr) {
        console.error("Error rendering head:", headErr);
      }
    }

    const result: RenderResult = { html, head };
    return new Response(JSON.stringify(result) + "\n");
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    return createError(`Failed to import component: ${message}`, err);
  }
}

async function handleBuild(req: Bun.BunRequest): Promise<Response> {
  let body: { entrypoints?: string[]; outdir?: string; target?: string };
  try {
    body = await req.json();
  } catch (err) {
    const message = err instanceof Error ? err.message : "Invalid JSON body";
    return createError(`Failed to parse request: ${message}`, err);
  }

  const { entrypoints, outdir, target } = body;

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
    const plugins = isSSR ? [] : [(await import("bun-plugin-tailwind")).default];

    const result = await Bun.build({
      entrypoints,
      outdir,
      target: buildTarget,
      minify: !isSSR,
      splitting: !isSSR,
      naming: isProduction
        ? {
            entry: "[name]-[hash].[ext]",
            chunk: "[name]-[hash].[ext]",
            asset: "[name]-[hash].[ext]",
          }
        : undefined,
      plugins,
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
