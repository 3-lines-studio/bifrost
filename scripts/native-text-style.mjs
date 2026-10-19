// Runs the pass that lets the text of a page inherit what a browser inherits and
// reach the font file that has the weight a rule asked for. It is generated as a
// string of Go and so is executed by no other test: the module is cut out of the
// generator and fed the shapes a stylesheet really produces -- a weight on one
// rule and the family on another, a weight with no file, a family the app does
// not ship, a style written as an array, and the same input twice.
//
// Run it with `make native-text-style`.

import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import path from "node:path";

const require = createRequire(import.meta.url);

const source = await readFile(new URL("../cmd/bifrost/native.go", import.meta.url), "utf8");
const marker = "const nativeTextStyle = `";
const start = source.indexOf(marker);
if (start < 0) throw new Error("nativeTextStyle is not in the generator");
const from = start + marker.length;
const to = source.indexOf("\n`", from);
if (to < 0) throw new Error("nativeTextStyle has no end");
const module = source.slice(from, to);

const directory = await mkdtemp(path.join(tmpdir(), "bifrost-text-"));
const file = path.join(directory, "text-style.js");
await writeFile(file, module);
const { inheritedText, textStyle } = require(file);

const families = { "Plus Jakarta Sans Variable": [400, 500, 600, 700] };

const cases = [
  [
    "el peso de una regla y la familia de otra",
    () => textStyle(inheritedText({}, [{ fontFamily: "Plus Jakarta Sans Variable" }]), families),
    (out) => out.fontFamily === "Plus Jakarta Sans Variable 400" && out.fontWeight === undefined,
  ],
  [
    "un peso de 500 llega al archivo de 500",
    () =>
      textStyle(
        inheritedText({ fontFamily: "Plus Jakarta Sans Variable" }, { fontWeight: 500 }),
        families,
      ),
    (out) => out.fontFamily === "Plus Jakarta Sans Variable 500" && out.fontWeight === undefined,
  ],
  [
    "la familia de la caja baja hasta el texto",
    () => inheritedText({ fontSize: 18 }, { fontWeight: "600" }),
    (out) => out.fontSize === 18 && out.fontWeight === "600",
  ],
  [
    "una familia que el texto escribe gana sobre la que bajo",
    () => inheritedText({ fontFamily: "Plus Jakarta Sans Variable" }, { fontFamily: "serif" }),
    (out) => out.fontFamily === "serif",
  ],
  [
    "bold y normal son 700 y 400",
    () => [textStyle({ fontFamily: "Plus Jakarta Sans Variable", fontWeight: "bold" }, families), textStyle({ fontFamily: "Plus Jakarta Sans Variable", fontWeight: "normal" }, families)],
    (out) => out[0].fontFamily.endsWith(" 700") && out[1].fontFamily.endsWith(" 400"),
  ],
  [
    "un peso que no es un numero no puede elegir archivo, y el inicial es 400",
    () => textStyle(inheritedText({}, { fontFamily: "Plus Jakarta Sans Variable", fontWeight: "var(--x)" }), families),
    (out) => out.fontFamily === "Plus Jakarta Sans Variable 400" && out.fontWeight === undefined,
  ],
  [
    "una familia que la app no trae queda como esta",
    () => textStyle({ fontFamily: "System", fontWeight: 700 }, families),
    (out) => out.fontFamily === "System" && out.fontWeight === 700,
  ],
  [
    "el peso que falta se resuelve al mas cercano, y el empate al mas pesado",
    () => [
      textStyle({ fontFamily: "A", fontWeight: 550 }, { A: [400, 700] }),
      textStyle({ fontFamily: "A", fontWeight: 900 }, { A: [400, 500] }),
    ],
    (out) => out[0].fontFamily === "A 700" && out[1].fontFamily === "A 500",
  ],
  [
    "lo que no se hereda no viaja",
    () => inheritedText({}, { margin: 8, paddingTop: 4, color: "red" }),
    (out) => out.margin === undefined && out.paddingTop === undefined && out.color === "red",
  ],
  [
    "un estilo escrito como arreglo se lee igual",
    () => inheritedText({}, [{ fontSize: 12 }, null, { color: "blue" }]),
    (out) => out.fontSize === 12 && out.color === "blue",
  ],
  [
    "correrlo dos veces es correrlo una",
    () => {
      const once = textStyle({ fontFamily: "Plus Jakarta Sans Variable", fontWeight: 500 }, families);
      return textStyle(once, families);
    },
    (out) => out.fontFamily === "Plus Jakarta Sans Variable 500" && out.fontWeight === undefined,
  ],
  [
    "la caja no se queda con lo que no es texto",
    () => textStyle(inheritedText({}, { fontFamily: "Plus Jakarta Sans Variable", margin: 8 }), families),
    (out) => out.margin === undefined && out.fontFamily === "Plus Jakarta Sans Variable 400",
  ],
];

let failed = 0;
for (const [name, run, check] of cases) {
  let out;
  try {
    out = run();
  } catch (error) {
    failed++;
    console.error(`FAIL ${name}: ${error.message}`);
    continue;
  }
  if (!check(out)) {
    failed++;
    console.error(`FAIL ${name}: ${JSON.stringify(out)}`);
    continue;
  }
  console.log(`ok ${name}`);
}

if (failed > 0) {
  console.error(`${failed} de ${cases.length} casos fallan`);
  process.exit(1);
}
console.log(`${cases.length} casos`);
