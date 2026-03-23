// Canonical NDJSON error + stream response helpers for prod renderers.
// Also inlined in react_prod.ts (stdin has no stable import root).
export interface ErrorDetail {
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

export function serializeError(error: unknown): {
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

export function createError(
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

/** One JSON line with head + html (Go RenderBodyStream and RenderChunked both accept this). */
export function singleLineRenderResponse(head: string, html: string): Response {
  return new Response(JSON.stringify({ head, html }) + "\n", {
    headers: {
      "Content-Type": "application/x-ndjson; charset=utf-8",
    },
  });
}

export function headThenRawStreamResponse(
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
