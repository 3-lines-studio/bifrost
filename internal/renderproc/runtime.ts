import { pathToFileURL } from "node:url";
import path from "node:path";

const socket = process.env.BIFROST_SOCKET;
if (!socket) throw new Error("BIFROST_SOCKET is required");

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
    const absolute = path.resolve(input.entry);
    const module = await import(pathToFileURL(absolute).href);
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
    const message = error instanceof Error ? error.stack ?? error.message : String(error);
    return errorResponse(message, 500);
  }
}

Bun.serve({
  unix: socket,
  routes: {
    "/health": () => new Response("ok"),
    "/render": { POST: handleRender },
  },
});
