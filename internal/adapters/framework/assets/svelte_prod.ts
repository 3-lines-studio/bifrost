const socket = process.env.BIFROST_SOCKET;

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

  try {
    const importPath = path.startsWith('/') ? 'file://' + path : path;
    const mod = await import(importPath);

    if (typeof mod.render !== "function") {
      return createError(
        `SSR bundle missing render function. Ensure the page was built with 'bifrost-build' and has an SSR bundle in .bifrost/ssr/. Path: ${path}`
      );
    }

    const result: RenderResult = await mod.render(props || {});
    return new Response(JSON.stringify(result) + "\n");
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    return createError(`Failed to render: ${message}`, err);
  }
}

Bun.serve({
  unix: socket,
  routes: {
    "/render": { POST: handleRender },
  },
});
