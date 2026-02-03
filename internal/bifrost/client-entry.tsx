import * as React from "react";
import { hydrateRoot } from "react-dom/client";
import * as Mod from "{{.ComponentImport}}";

const root = document.getElementById("app");
if (!root) {
  throw new Error("Missing #app element");
}

const propsScript = document.getElementById("__BIFROST_PROPS__");
const propsText = propsScript?.textContent;
const props = propsText ? JSON.parse(propsText) : {};

const Component =
  Mod.default ||
  Mod.Page ||
  Object.values(Mod).find((x: any) => typeof x === "function");
if (!Component) {
  throw new Error("No component export found in {{.ComponentImport}}");
}

const doHydrate = () => hydrateRoot(root, <Component {...props} />);

if ("requestIdleCallback" in window) {
  requestIdleCallback(doHydrate, { timeout: 2000 });
} else {
  setTimeout(doHydrate, 0);
}
