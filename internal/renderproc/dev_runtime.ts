import { createServer } from "vite";

const socket = process.env.BIFROST_SOCKET;
const root = process.env.BIFROST_VITE_ROOT;
const port = Number(process.env.BIFROST_VITE_PORT || "5173");
if (!socket) throw new Error("BIFROST_SOCKET is required");
if (!root) throw new Error("BIFROST_VITE_ROOT is required");

const vite = await createServer({
  root,
  appType: "custom",
  server: {
    host: "127.0.0.1",
    port,
    strictPort: true,
    cors: true,
  },
  resolve: {
    dedupe: ["react", "react-dom"],
  },
});
await vite.listen();
vite.printUrls();

const encoder = new TextEncoder();
const HEAD = 1;
const BODY = 2;
const DONE = 3;
const ERROR = 4;

function frame(kind: number, payload?: Uint8Array | string): Uint8Array {
  const data = typeof payload === "string" ? encoder.encode(payload) : payload ?? new Uint8Array();
  const output = new Uint8Array(5 + data.length);
  output[0] = kind;
  new DataView(output.buffer).setUint32(1, data.length, false);
  output.set(data, 5);
  return output;
}

function errorResponse(message: string, status: number): Response {
  return new Response(frame(ERROR, message), {
    status,
    headers: { "content-type": "application/octet-stream" },
  });
}

async function handleRender(request: Request): Promise<Response> {
  let input: { entry?: string; props?: unknown };
  try {
    input = await request.json();
  } catch (error) {
    return errorResponse(String(error), 400);
  }
  if (!input.entry) return errorResponse("missing entry", 400);

  try {
    const module = await vite.ssrLoadModule(input.entry);
    if (typeof module.render !== "function") {
      throw new Error(`module ${input.entry} does not export render`);
    }
    const rendered = await module.render(input.props ?? {}, request.signal);
    const head = typeof rendered.head === "string" ? rendered.head : "";
    const body: ReadableStream<Uint8Array> = rendered.body;
    if (!body || typeof body.getReader !== "function") {
      throw new Error(`module ${input.entry} returned an invalid body stream`);
    }

    let reader: ReadableStreamDefaultReader<Uint8Array> | undefined;
    return new Response(new ReadableStream<Uint8Array>({
      async start(controller) {
        controller.enqueue(frame(HEAD, head));
        reader = body.getReader();
        const abort = () => { void reader?.cancel(request.signal.reason); };
        if (request.signal.aborted) abort();
        else request.signal.addEventListener("abort", abort, { once: true });
        try {
          for (;;) {
            const result = await reader.read();
            if (result.done) break;
            if (result.value.length) controller.enqueue(frame(BODY, result.value));
          }
          if (!request.signal.aborted) {
            controller.enqueue(frame(DONE));
            controller.close();
          }
        } catch (error) {
          if (!request.signal.aborted) {
            controller.enqueue(frame(ERROR, error instanceof Error ? error.message : String(error)));
            controller.close();
          }
        } finally {
          request.signal.removeEventListener("abort", abort);
        }
      },
      cancel(reason) {
        return reader?.cancel(reason);
      },
    }), { headers: { "content-type": "application/octet-stream" } });
  } catch (error) {
    vite.ssrFixStacktrace(error as Error);
    const message = error instanceof Error ? error.stack ?? error.message : String(error);
    return errorResponse(message, 500);
  }
}

const renderer = Bun.serve({
  unix: socket,
  routes: {
    "/health": () => new Response("ok"),
    "/render": { POST: handleRender },
  },
});

async function close() {
  renderer.stop(true);
  await vite.close();
  process.exit(0);
}
process.on("SIGTERM", close);
process.on("SIGINT", close);
