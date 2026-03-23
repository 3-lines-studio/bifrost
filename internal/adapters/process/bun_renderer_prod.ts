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

  try {
    const importPath = path.startsWith('/') ? 'file://' + path : path;
    const mod = await import(importPath);

    if (typeof mod.render !== "function") {
      return createError(
        `SSR bundle missing render function. Ensure the page was built with 'bifrost-build' and has an SSR bundle in .bifrost/ssr/. Path: ${path}`
      );
    }

    const result: RenderResult = await mod.render(props || {}, {
      streamBody: wantStream,
    });
    if (result.stream instanceof ReadableStream) {
      return headThenRawStreamResponse(result.head ?? "", result.stream);
    }
    return ndjsonRenderResponse(result.head ?? "", result.html ?? "");
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
