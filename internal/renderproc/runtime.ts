import { pathToFileURL } from "node:url";
import path from "node:path";
import type { Socket } from "bun";

const socket = process.env.BIFROST_SOCKET;
if (!socket) throw new Error("BIFROST_SOCKET is required");

const HEAD = 1;
const BODY = 2;
const DONE = 3;
const ERROR = 4;
const maxRequestBytes = 16 << 20;
const encoder = new TextEncoder();
const decoder = new TextDecoder();
const done = frame(DONE);

type State = {
  buffer: Uint8Array;
  queue: Promise<void>;
  abort?: AbortController;
  drain?: () => void;
  closed: boolean;
};

function frame(kind: number, payload?: Uint8Array | string): Uint8Array {
  const data = typeof payload === "string" ? encoder.encode(payload) : payload ?? new Uint8Array();
  const output = new Uint8Array(5 + data.length);
  output[0] = kind;
  new DataView(output.buffer).setUint32(1, data.length, false);
  output.set(data, 5);
  return output;
}

async function send(ws: Socket<State>, state: State, data: Uint8Array) {
  let rest = data;
  while (rest.length > 0) {
    if (state.closed) throw new Error("renderer socket closed");
    const wrote = ws.write(rest);
    if (wrote >= rest.length) return;
    rest = rest.subarray(wrote);
    await new Promise<void>((resolve) => { state.drain = resolve; });
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.stack ?? error.message : String(error);
}

async function render(ws: Socket<State>, state: State, payload: Uint8Array) {
  const controller = new AbortController();
  state.abort = controller;
  try {
    let input: { entry?: string; props?: unknown };
    try {
      input = JSON.parse(decoder.decode(payload));
    } catch (error) {
      await send(ws, state, frame(ERROR, errorMessage(error)));
      return;
    }
    if (!input.entry) {
      await send(ws, state, frame(ERROR, "missing entry"));
      return;
    }
    const absolute = path.resolve(input.entry);
    let module = modules.get(absolute);
    if (!module) {
      module = await import(pathToFileURL(absolute).href);
      modules.set(absolute, module);
    }
    if (typeof module.render !== "function") {
      throw new Error(`module ${input.entry} does not export render`);
    }
    const rendered = await module.render(input.props ?? {}, controller.signal);
    const head = typeof rendered.head === "string" ? rendered.head : "";
    const body: ReadableStream<Uint8Array> = rendered.body;
    if (!body || typeof body.getReader !== "function") {
      throw new Error(`module ${input.entry} returned an invalid body stream`);
    }
    const reader = body.getReader();
    const abort = () => { void reader.cancel(controller.signal.reason); };
    if (controller.signal.aborted) abort();
    else controller.signal.addEventListener("abort", abort, { once: true });
    try {
      await send(ws, state, frame(HEAD, head));
      for (;;) {
        const result = await reader.read();
        if (result.done) break;
        if (result.value.length) await send(ws, state, frame(BODY, result.value));
      }
      if (!state.closed) await send(ws, state, done);
    } finally {
      controller.signal.removeEventListener("abort", abort);
    }
  } catch (error) {
    if (!state.closed) {
      try { await send(ws, state, frame(ERROR, errorMessage(error))); } catch { state.closed = true; }
    }
  } finally {
    state.abort = undefined;
  }
}

const modules = new Map<string, any>();

Bun.listen<State>({
  unix: socket,
  socket: {
    open(ws) {
      ws.data = { buffer: new Uint8Array(0), queue: Promise.resolve(), closed: false };
    },
    data(ws, chunk) {
      const state = ws.data;
      if (state.closed) return;
      const merged = new Uint8Array(state.buffer.length + chunk.length);
      merged.set(state.buffer, 0);
      merged.set(chunk, state.buffer.length);
      state.buffer = merged;
      for (;;) {
        if (state.buffer.length < 4) return;
        const size = new DataView(state.buffer.buffer, state.buffer.byteOffset, 4).getUint32(0, false);
        if (size > maxRequestBytes) {
          state.closed = true;
          ws.close();
          return;
        }
        if (state.buffer.length - 4 < size) return;
        const payload = state.buffer.subarray(4, 4 + size);
        state.buffer = state.buffer.slice(4 + size);
        state.queue = state.queue.then(() => (size === 0 ? send(ws, state, done) : render(ws, state, payload)));
      }
    },
    drain(ws) {
      const resolve = ws.data.drain;
      ws.data.drain = undefined;
      resolve?.();
    },
    close(ws) {
      const state = ws.data;
      state.closed = true;
      const resolve = state.drain;
      state.drain = undefined;
      resolve?.();
      state.abort?.abort();
    },
    error(ws) {
      ws.data.closed = true;
    },
  },
});
