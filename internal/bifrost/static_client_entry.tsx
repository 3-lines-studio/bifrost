import * as React from "react";
import { createRoot } from "react-dom/client";
import * as Mod from "{{.ComponentImport}}";

const root = document.getElementById("app");
if (!root) {
  throw new Error("Missing #app element");
}

const Component =
  Mod.default ||
  Mod.Page ||
  Object.values(Mod).find((x: any) => typeof x === "function");
if (!Component) {
  throw new Error("No component export found in {{.ComponentImport}}");
}

const doRender = () => {
  createRoot(root).render(<Component />);
};

if ("requestIdleCallback" in window) {
  requestIdleCallback(doRender, { timeout: 2000 });
} else {
  setTimeout(doRender, 0);
}
