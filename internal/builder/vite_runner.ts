import path from "node:path";
import { build, type Plugin } from "vite";

type Target = {
  entries: Record<string, string>;
  outDir: string;
  manifest: string;
};

type Request = {
  root: string;
  sourceMaps: boolean;
  client: Target;
  server?: Target;
};

const request = (await Bun.stdin.json()) as Request;

function bifrostGuard(outDir: string, ssr: boolean): Plugin {
  return {
    name: "bifrost:invariant-guard",
    enforce: "post",
    configResolved(config) {
      if (config.base !== "/_bifrost/dist/") {
        throw new Error(`Bifrost requires Vite base /_bifrost/dist/, got ${config.base}`);
      }
      if (path.resolve(config.build.outDir) !== path.resolve(outDir)) {
        throw new Error(`Bifrost requires Vite output ${outDir}, got ${config.build.outDir}`);
      }
      if (Boolean(config.build.ssr) !== ssr) {
        throw new Error(`Bifrost Vite SSR mode was changed by user configuration`);
      }
    },
  };
}

const shared = {
  root: request.root,
  base: "/_bifrost/dist/",
  logLevel: "warn" as const,
};

await build({
  ...shared,
  plugins: [bifrostGuard(request.client.outDir, false)],
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
});

if (request.server && Object.keys(request.server.entries).length > 0) {
  await build({
    ...shared,
    plugins: [bifrostGuard(request.server.outDir, true)],
    build: {
      outDir: request.server.outDir,
      emptyOutDir: false,
      copyPublicDir: false,
      manifest: request.server.manifest,
      ssr: true,
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
    ssr: {
      noExternal: true,
    },
  });
}
