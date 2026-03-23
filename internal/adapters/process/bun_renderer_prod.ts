import {
  createError,
  headThenRawStreamResponse,
  singleLineRenderResponse,
} from "../framework/assets/render_protocol";

const socket = process.env.BIFROST_SOCKET;

interface RenderResult {
  html?: string;
  head?: string;
  stream?: ReadableStream<Uint8Array>;
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
    const importPath = path.startsWith("/") ? "file://" + path : path;
    const mod = await import(importPath);

    if (typeof mod.render !== "function") {
      return createError(
        `SSR bundle missing render function. Ensure the page was built with 'bifrost-build' and has an SSR bundle in .bifrost/ssr/. Path: ${path}`,
      );
    }

    const result: RenderResult = await mod.render(props || {}, {
      streamBody: wantStream,
    });
    if (result.stream instanceof ReadableStream) {
      return headThenRawStreamResponse(result.head ?? "", result.stream);
    }
    return singleLineRenderResponse(result.head ?? "", result.html ?? "");
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
