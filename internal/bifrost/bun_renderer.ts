import { renderToString } from "react-dom/server";
import React from "react";
import tailwind from "bun-plugin-tailwind";

const socket = process.env.BIFROST_SOCKET;
const isDev =
  process.env.BIFROST_DEV === "1" || process.env.BIFROST_DEV === "true";

if (!socket) {
  console.error("BIFROST_SOCKET environment variable not set");
  process.exit(1);
}

const componentCache = new Map<
  string,
  { Component: React.ComponentType; Head?: React.ComponentType }
>();

async function handleRender(req: Bun.BunRequest) {
  const body = await req.json();
  const { path, props } = body;

  if (!path) {
    return new Response(
      JSON.stringify({ error: "Missing 'path' in request" }) + "\n",
      { status: 400 },
    );
  }

  const importPath = isDev ? `${path}?t=${Date.now()}` : path;

  let Component: React.ComponentType;
  let Head: React.ComponentType | undefined;

  const cached = componentCache.get(path);
  if (!isDev && cached) {
    Component = cached.Component;
    Head = cached.Head;
  } else {
    const mod = await import(importPath);
    Component =
      mod.default ||
      mod.Page ||
      Object.values(mod).find((x) => typeof x === "function");
    Head = mod.Head;

    if (!isDev && Component) {
      componentCache.set(path, { Component, Head });
    }
  }

  if (!Component) {
    return new Response(
      JSON.stringify({
        error: `No component export found in ${path}. Expected default export, Page export, or a function export.`,
      }) + "\n",
      { status: 500 },
    );
  }

  const componentProps = props || {};

  let html;
  try {
    const el = React.createElement(Component, componentProps);
    html = renderToString(el);
  } catch (renderErr) {
    const errorMessage =
      renderErr instanceof Error ? renderErr.message : String(renderErr);
    const errorStack = renderErr instanceof Error ? renderErr.stack : "";
    return new Response(
      JSON.stringify({
        error: errorMessage,
        stack: errorStack,
      }) + "\n",
      { status: 500 },
    );
  }

  let head = "";
  if (Head) {
    try {
      const headEl = React.createElement(Head, componentProps);
      head = renderToString(headEl);
    } catch (headErr) {
      console.error("Error rendering head:", headErr);
    }
  }

  return new Response(JSON.stringify({ html, head }) + "\n");
}

async function handleBuild(req: Bun.BunRequest) {
  const body = await req.json();
  const { entrypoints, outdir } = body;

  if (!Array.isArray(entrypoints) || entrypoints.length === 0) {
    return new Response(
      JSON.stringify({ error: "Missing entrypoints" }) + "\n",
      {
        status: 400,
      },
    );
  }

  if (!outdir) {
    return new Response(JSON.stringify({ error: "Missing outdir" }) + "\n", {
      status: 400,
    });
  }

  const isProduction =
    process.env.BIFROST_PROD === "1" || process.env.BIFROST_PROD === "true";

  const result = await Bun.build({
    entrypoints,
    outdir,
    target: "browser",
    minify: true,
    splitting: true,
    naming: isProduction
      ? {
          entry: "[name]-[hash].[ext]",
          chunk: "[name]-[hash].[ext]",
          asset: "[name]-[hash].[ext]",
        }
      : undefined,
    plugins: [tailwind],
  });

  if (!result.success) {
    return new Response(
      JSON.stringify({ error: "Failed to build client bundle" }) + "\n",
      { status: 500 },
    );
  }

  return new Response(JSON.stringify({ ok: true }) + "\n");
}

Bun.serve({
  unix: socket,
  routes: {
    "/render": { POST: handleRender },
    "/build": { POST: handleBuild },
  },
});

console.log(`[bifrost] Renderer ready on ${socket}`);
