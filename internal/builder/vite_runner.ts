import path from "node:path";
import { createBuilder, type Plugin } from "vite";

type Target = {
  entries: Record<string, string>;
  outDir: string;
  manifest: string;
};

type RouteSpec = {
  pattern: string;
  view: string;
  kind: string;
};

type Request = {
  root: string;
  sourceMaps: boolean;
  configFile?: string;
  routes?: RouteSpec[];
  client: Target;
  server?: Target;
};

const request = (await Bun.stdin.json()) as Request;

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

function bifrostRoutes(routes: RouteSpec[]): Plugin {
  const id = "virtual:bifrost/routes";
  const resolved = "\0" + id;
  return {
    name: "bifrost:routes",
    resolveId(source) {
      if (source === "virtual:bifrost/navigation") {
        return path.join(import.meta.dirname, "navigation-api.ts");
      }
      return source === id ? resolved : null;
    },
    load(source) {
      return source === resolved ? generateRoutesModule(routes) : null;
    },
  };
}

function bifrostGuard(clientOutDir: string, serverOutDir?: string): Plugin {
  return {
    name: "bifrost:invariant-guard",
    enforce: "post",
    configResolved(config) {
      if (config.base !== "/_bifrost/dist/") {
        throw new Error(`Bifrost requires Vite base /_bifrost/dist/, got ${config.base}`);
      }
      const ssr = Boolean(config.build.ssr);
      const expected = ssr ? serverOutDir : clientOutDir;
      if (!expected) {
        throw new Error("Bifrost Vite SSR mode was changed by user configuration");
      }
      const environmentOutDir = config.environments?.[ssr ? "ssr" : "client"]?.build?.outDir;
      const isEnvironmentResolution = environmentOutDir !== undefined && path.resolve(environmentOutDir) === path.resolve(config.build.outDir);
      if (!isEnvironmentResolution) {
        return;
      }
      if (path.resolve(config.build.outDir) !== path.resolve(expected)) {
        throw new Error(`Bifrost requires Vite output ${expected}, got ${config.build.outDir}`);
      }
    },
  };
}

const shared = {
  root: request.root,
  configFile: request.configFile,
  base: "/_bifrost/dist/",
  logLevel: "warn" as const,
  resolve: {
    dedupe: ["react", "react-dom"],
  },
};

const builder = await createBuilder({
  ...shared,
  builder: {},
  plugins: [bifrostRoutes(request.routes ?? []), bifrostGuard(request.client.outDir, request.server?.outDir)],
  environments: {
    client: {
      build: {
        outDir: request.client.outDir,
        emptyOutDir: false,
        copyPublicDir: false,
        manifest: request.client.manifest,
        sourcemap: request.sourceMaps ? "inline" : false,
        minify: true,
        assetsInlineLimit: 0,
        chunkSizeWarningLimit: 5000,
        rollupOptions: {
          input: request.client.entries,
          output: {
            entryFileNames: "assets/[name]-[hash].js",
            chunkFileNames: "assets/chunk-[name]-[hash].js",
            assetFileNames: "assets/[name]-[hash][extname]",
          },
        },
      },
    },
    ...(request.server && Object.keys(request.server.entries).length > 0
      ? {
          ssr: {
            resolve: {
              noExternal: true,
            },
            build: {
              outDir: request.server.outDir,
              emptyOutDir: false,
              copyPublicDir: false,
              manifest: request.server.manifest,
              sourcemap: request.sourceMaps ? "inline" : false,
              minify: true,
              assetsInlineLimit: 0,
              chunkSizeWarningLimit: 5000,
              rollupOptions: {
                input: request.server.entries,
                output: {
                  entryFileNames: "[name]-[hash].js",
                  chunkFileNames: "chunks/[name]-[hash].js",
                  assetFileNames: "assets/[name]-[hash][extname]",
                },
              },
            },
          },
        }
      : {}),
  },
});

await builder.buildApp();
