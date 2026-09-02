import { createServer, type ModuleNode, type Plugin } from "vite";
import { readFileSync, readdirSync, watch } from "node:fs";
import path from "node:path";

const socket = process.env.BIFROST_SOCKET;
const root = process.env.BIFROST_VITE_ROOT;
const port = Number(process.env.BIFROST_VITE_PORT || "5173");
const routesFile = process.env.BIFROST_ROUTES_FILE;
const viteConfig = process.env.BIFROST_VITE_CONFIG || undefined;
const devEntries = process.env.BIFROST_DEV_ENTRIES || undefined;
if (!socket) throw new Error("BIFROST_SOCKET is required");
if (!root) throw new Error("BIFROST_VITE_ROOT is required");

type RouteSpec = { pattern: string; view: string; kind: string };

function loadRoutes(): RouteSpec[] {
  if (!routesFile) return [];
  try {
    const parsed: unknown = JSON.parse(readFileSync(routesFile, "utf8"));
    return Array.isArray(parsed) ? (parsed as RouteSpec[]) : [];
  } catch {
    return [];
  }
}

function generateRoutesModule(routes: RouteSpec[]): string {
  return [
    `const routes = ${JSON.stringify(routes)};`,
    "export function href(pattern, params = {}) {",
    "  if (pattern === '/{$}') return '/';",
    "  const origin = 'http://placeholder';",
    "  const url = new URL(pattern.replace(/\\{([^}]+)\\}/g, (whole, name) => {",
    "    if (name === '$') return '';",
    "    const value = params[name];",
    "    if (value === undefined) throw new Error(`missing route param ${name} for pattern ${pattern}`);",
    "    const raw = Array.isArray(value) ? value.join('/') : String(value);",
    "    return raw.split('/').map(encodeURIComponent).join('/');",
    "  }), origin);",
    "  return url.pathname + url.search;",
    "}",
    "export { routes };",
    "",
  ].join("\n");
}

function bifrostRoutesPlugin(): Plugin {
  const id = "virtual:bifrost/routes";
  const resolved = "\0" + id;
  return {
    name: "bifrost:routes",
    resolveId(source) {
      return source === id ? resolved : null;
    },
    load(source) {
      return source === resolved ? generateRoutesModule(loadRoutes()) : null;
    },
  };
}

const vite = await createServer({
  root,
  configFile: viteConfig,
  appType: "custom",
  base: "/_bifrost/dev/",
  plugins: [bifrostRoutesPlugin()],
  server: {
    host: "127.0.0.1",
    port,
    strictPort: true,
    cors: false,
    ws: { clientPort: 0 },
    watch: devEntries
      ? { ignored: [devEntries, path.join(devEntries, "**")] }
      : undefined,
  },
  resolve: {
    dedupe: ["react", "react-dom"],
  },
});
await vite.listen();
vite.printUrls();

if (devEntries) {
  try {
    for (const name of readdirSync(devEntries)) {
      if (!name.endsWith("-client.tsx")) continue;
      vite.warmupRequest(`/@fs${path.join(devEntries, name)}`).catch(() => {});
    }
  } catch {}
}

if (routesFile) {
  watch(path.dirname(routesFile), (_event, filename) => {
    if (!filename || !routesFile.endsWith(filename)) return;
    vite.moduleGraph.invalidateAll();
    vite.hot.send({ type: "full-reload" });
  });
}

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

const devBase = vite.config.base.replace(/\/$/, "");

// Collects the CSS modules reachable from a server entry in Vite's live SSR
// module graph. The dev bridge prepends stylesheet links for them so SSR
// responses carry their styles and hydrated pages do not flash unstyled
// content while Vite's client injects CSS. CSS imported by other CSS modules
// (for example Tailwind's library CSS behind an @import) is excluded: Vite
// serves the entry stylesheet with its @imports intact.
async function collectSsrCss(entryUrl: string): Promise<string[]> {
  const seen = new Set<string>();
  const isCss = (url: string): boolean => /\.css($|\?)/.test(url);
  const cssUrls = new Set<string>();
  const importedByCss = new Set<string>();
  const visit = (node: ModuleNode | undefined): void => {
    if (!node || seen.has(node.url)) return;
    seen.add(node.url);
    const css = isCss(node.url);
    if (css) cssUrls.add(node.url);
    for (const dep of node.ssrImportedModules) {
      if (css && isCss(dep.url)) importedByCss.add(dep.url);
      visit(dep);
    }
  };
  visit(await vite.moduleGraph.getModuleByUrl(entryUrl, true));
  return [...cssUrls]
    .filter((url) => !importedByCss.has(url))
    .map((url) => devBase + url + (url.includes("?") ? "&direct" : "?direct"));
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
    const cssLinks = (await collectSsrCss(input.entry))
      .map((url) => `<link rel="stylesheet" href="${url}">`)
      .join("") + head;
    const body: ReadableStream<Uint8Array> = rendered.body;
    if (!body || typeof body.getReader !== "function") {
      throw new Error(`module ${input.entry} returned an invalid body stream`);
    }

    let reader: ReadableStreamDefaultReader<Uint8Array> | undefined;
    return new Response(new ReadableStream<Uint8Array>({
      async start(controller) {
        controller.enqueue(frame(HEAD, cssLinks));
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
    if (!request.signal.aborted) {
      vite.hot.send({ type: "error", err: { message, stack: message } });
    }
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
