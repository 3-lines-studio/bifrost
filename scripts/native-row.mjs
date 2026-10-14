// Runs the pass that gives a flex container the direction a browser gives it.
// It is generated as a string of Go and so is executed by no other test. The
// module is cut out of the generator and fed the shapes the compiler really
// emits: a rule with the display and nothing else, the inline form, a rule that
// already names a direction, a declaration that arrives through a variable, a
// rule with no display at all, and the same stylesheet twice.
//
// Run it with `make native-row`.

import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import path from "node:path";

const require = createRequire(import.meta.url);

const source = await readFile(new URL("../cmd/bifrost/native.go", import.meta.url), "utf8");
const marker = "const nativeRowDirection = `";
const start = source.indexOf(marker);
if (start < 0) throw new Error("nativeRowDirection is not in the generator");
const from = start + marker.length;
const to = source.indexOf("\n`", from);
if (to < 0) throw new Error("nativeRowDirection has no end");
const module = source.slice(from, to);

const directory = await mkdtemp(path.join(tmpdir(), "bifrost-row-"));
const file = path.join(directory, "row-direction.js");
await writeFile(file, module);
const { withRowDirection } = require(file);

const rule = (order, declarations) => ({ s: [order, 1], d: [declarations] });
const sheet = (rules) => ({ s: rules });
const declarations = (stylesheet, name) => {
  const entry = stylesheet.s.find((one) => one[0] === name);
  return entry ? entry[1][0] : undefined;
};

const cases = [
  [
    "una regla con flex",
    sheet([["flex", [rule(54, { display: "flex" })]]]),
    (out) => declarations(out, "flex").d[0].flexDirection === "row",
  ],
  [
    "la forma inline, que el compilador tira",
    sheet([["inline-flex", [rule(55, { display: "inline-flex" })]]]),
    (out) => declarations(out, "inline-flex").d[0].flexDirection === "row",
  ],
  [
    "una regla que ya dice su direccion no se toca",
    sheet([["flex-col", [rule(58, { display: "flex", flexDirection: "column" })]]]),
    (out) =>
      declarations(out, "flex-col").d[0].flexDirection === "column" &&
      Object.keys(declarations(out, "flex-col").d[0]).length === 2,
  ],
  [
    "una direccion que llega por variable no se pisa",
    sheet([["x", [rule(60, [[{}, "var", "--d", 1], "flexDirection", 1])]]]),
    (out) => declarations(out, "x").d.length === 1 && Array.isArray(declarations(out, "x").d[0]),
  ],
  [
    "una declaracion con variable no se lee como el display",
    sheet([["x", [rule(61, [[{}, "var", "--d", 1], "display", 1])]]]),
    (out) => declarations(out, "x").d.length === 1 && Array.isArray(declarations(out, "x").d[0]),
  ],
  [
    "block no es flex",
    sheet([["x", [rule(62, { display: "block" })]]]),
    (out) => declarations(out, "x").d[0].flexDirection === undefined,
  ],
  [
    "el orden de las reglas no se toca, asi el de despues sigue ganando",
    sheet([
      ["flex", [rule(54, { display: "flex" })]],
      ["flex-col", [rule(58, { flexDirection: "column" })]],
    ]),
    (out) =>
      out.s.map((one) => one[0]).join(",") === "flex,flex-col" &&
      declarations(out, "flex").d[0].flexDirection === "row" &&
      declarations(out, "flex-col").d[0].flexDirection === "column" &&
      declarations(out, "flex").s[0] === 54,
  ],
  [
    "correrlo dos veces es correrlo una",
    sheet([["flex", [rule(54, { display: "flex" })]]]),
    (out) => Object.keys(declarations(withRowDirection(out), "flex").d[0]).length === 2,
  ],
  [
    "sin coleccion no tira",
    {},
    (out) => out !== null && out !== undefined,
  ],
  [
    "una coleccion sin reglas no tira",
    sheet([["flex", []]]),
    (out) => Array.isArray(out.s),
  ],
];

let failed = 0;
for (const [name, input, check] of cases) {
  const before = JSON.stringify(input);
  let out;
  try {
    out = withRowDirection(input);
  } catch (error) {
    failed += 1;
    console.log("FALLA " + name + ": tiró " + error.message);
    continue;
  }
  if (out !== input) {
    failed += 1;
    console.log("FALLA " + name + ": no devolvió la misma colección");
    continue;
  }
  if (!check(out)) {
    failed += 1;
    console.log("FALLA " + name);
    console.log("  entrada  " + before);
    console.log("  obtenido " + JSON.stringify(out));
  }
}
console.log(failed === 0 ? cases.length + " casos, todos bien" : failed + " de " + cases.length + " fallan");
process.exit(failed === 0 ? 0 : 1);
