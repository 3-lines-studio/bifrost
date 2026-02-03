import { renderToString } from "react-dom/server";
import React from "react";
import tailwind from "bun-plugin-tailwind";

const socket = process.env.BIFROST_SOCKET;
const isDev =
  process.env.BIFROST_DEV === "1" || process.env.BIFROST_DEV === "true";

const componentCache = new Map<
  string,
  { Component: React.ComponentType; Head?: React.ComponentType }
>();

interface RenderResult {
  html?: string;
  head?: string;
  error?: {
    message: string;
    stack?: string;
  };
}

interface BuildResult {
  ok?: boolean;
  error?: {
    message: string;
    stack?: string;
  };
}

function createErrorResponse(message: string, stack?: string): Response {
  const result: RenderResult = {
    error: {
      message,
      stack,
    },
  };
  return new Response(JSON.stringify(result) + "\n");
}

function createBuildErrorResponse(message: string): Response {
  const result: BuildResult = {
    error: {
      message,
    },
  };
  return new Response(JSON.stringify(result) + "\n");
}

async function handleRender(req: Bun.BunRequest): Promise<Response> {
  let body: { path?: string; props?: Record<string, unknown> };
  try {
    body = await req.json();
  } catch (err) {
    const message = err instanceof Error ? err.message : "Invalid JSON body";
    return createErrorResponse(`Failed to parse request: ${message}`);
  }

  const { path, props } = body;

  if (!path) {
    return createErrorResponse("Missing 'path' in request");
  }

  const importPath = isDev ? `${path}?t=${Date.now()}` : path;

  let Component: React.ComponentType;
  let Head: React.ComponentType | undefined;

  try {
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
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    const stack = err instanceof Error ? err.stack : undefined;
    return createErrorResponse(`Failed to import component: ${message}`, stack);
  }

  if (!Component) {
    return createErrorResponse(
      `No component export found in ${path}. Expected default export, Page export, or a function export.`,
    );
  }

  const componentProps = props || {};

  let html: string;
  try {
    const el = React.createElement(Component, componentProps);
    html = renderToString(el);
  } catch (renderErr) {
    const message =
      renderErr instanceof Error ? renderErr.message : String(renderErr);
    const stack = renderErr instanceof Error ? renderErr.stack : undefined;
    return createErrorResponse(`Render error: ${message}`, stack);
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

  const result: RenderResult = { html, head };
  return new Response(JSON.stringify(result) + "\n");
}

async function handleBuild(req: Bun.BunRequest): Promise<Response> {
  let body: { entrypoints?: string[]; outdir?: string };
  try {
    body = await req.json();
  } catch (err) {
    const message = err instanceof Error ? err.message : "Invalid JSON body";
    return createBuildErrorResponse(`Failed to parse request: ${message}`);
  }

  const { entrypoints, outdir } = body;

  if (!Array.isArray(entrypoints) || entrypoints.length === 0) {
    return createBuildErrorResponse("Missing entrypoints");
  }

  if (!outdir) {
    return createBuildErrorResponse("Missing outdir");
  }

  const isProduction =
    process.env.BIFROST_PROD === "1" || process.env.BIFROST_PROD === "true";

  try {
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
      const errors = result.logs
        .filter((log) => log.level === "error")
        .map((log) => log.message)
        .join("\n");
      return createBuildErrorResponse(`Build failed: ${errors}`);
    }

    const response: BuildResult = { ok: true };
    return new Response(JSON.stringify(response) + "\n");
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    return createBuildErrorResponse(`Build error: ${message}`);
  }
}

Bun.serve({
  unix: socket,
  routes: {
    "/render": { POST: handleRender },
    "/build": { POST: handleBuild },
  },
});
