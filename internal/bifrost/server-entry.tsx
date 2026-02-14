import * as React from "react";
import { renderToString } from "react-dom/server";
import * as Mod from "{{.ComponentImport}}";

const Component =
  Mod.default ||
  Mod.Page ||
  Object.values(Mod).find((x: any) => typeof x === "function");

const Head = Mod.Head;

export function render(props: Record<string, unknown>): { html: string; head: string } {
  if (!Component) {
    throw new Error("No component export found in {{.ComponentImport}}");
  }

  const el = React.createElement(Component, props);
  const html = renderToString(el);

  let head = "";
  if (Head) {
    try {
      const headEl = React.createElement(Head, props);
      head = renderToString(headEl);
    } catch (headErr) {
      console.error("Error rendering head:", headErr);
    }
  }

  return { html, head };
}
