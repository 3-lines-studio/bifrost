// Runs the rewrite that turns a position a native view cannot express into the
// one it can, which is generated as a string of Go and so is executed by no
// other test. The module is cut out of the generator and run over the
// declarations a stylesheet really has: the pinned box the shell of an app
// writes, the value that already is native, a value that comes from a variable,
// a declaration that is not a position, and the same rewrite twice.
//
// Run it with `make native-position`.

import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import path from "node:path";

const require = createRequire(import.meta.url);

const source = await readFile(new URL("../cmd/bifrost/native.go", import.meta.url), "utf8");
const marker = "const nativePosition = `";
const start = source.indexOf(marker);
if (start < 0) throw new Error("nativePosition is not in the generator");
const from = start + marker.length;
const to = source.indexOf("\n`", from);
if (to < 0) throw new Error("nativePosition has no end");
const module = source.slice(from, to);

const directory = await mkdtemp(path.join(tmpdir(), "bifrost-position-"));
const file = path.join(directory, "position.js");
await writeFile(file, module);
const { withFixed } = require(file);

const norm = (css) => css.replace(/\s+/g, " ").trim();

function run(css) {
  return norm(withFixed(css));
}

const cases = [
  [
    "la caja que la pagina clava en la ventana",
    ".fixed { position: fixed; top: 0; left: 0; right: 0 }",
    ".fixed { position: absolute; top: 0; left: 0; right: 0 }",
  ],
  [
    "el sticky no se toca: lo contesta la vista que scrollea",
    ".sticky { position: sticky; top: 0 }",
    ".sticky { position: sticky; top: 0 }",
  ],
  [
    "lo que ya es nativo vuelve igual",
    ".absolute { position: absolute }",
    ".absolute { position: absolute }",
  ],
  [
    "una posicion que viene de una variable no se adivina",
    ".thing { position: var(--where) }",
    ".thing { position: var(--where) }",
  ],
  [
    "una declaracion que no es una posicion no se toca",
    ".a { background-position: fixed }",
    ".a { background-position: fixed }",
  ],
  [
    "adentro de una media query tambien",
    "@media (min-width: 768px) { .fixed { position: fixed } }",
    "@media (min-width: 768px) { .fixed { position: absolute } }",
  ],
  [
    "con una propiedad del tema al lado",
    ".fixed { --x: 1; position: fixed }",
    ".fixed { --x: 1; position: absolute }",
  ],
];

let failed = 0;
for (const [name, input, wanted] of cases) {
  let got;
  try {
    got = run(input);
  } catch (error) {
    failed += 1;
    console.log("FALLA " + name + ": tiró " + error.message);
    continue;
  }
  if (got !== wanted) {
    failed += 1;
    console.log("FALLA " + name);
    console.log("  entrada  " + JSON.stringify(input));
    console.log("  esperado " + JSON.stringify(wanted));
    console.log("  obtenido " + JSON.stringify(got));
  }
}

if (failed === 0) {
  const twice = norm(withFixed(withFixed(".fixed { position: fixed }")));
  if (twice !== ".fixed { position: absolute }") {
    failed += 1;
    console.log("FALLA correrlo dos veces: " + JSON.stringify(twice));
  }
}
console.log(failed === 0 ? cases.length + " casos, todos bien" : failed + " de " + cases.length + " fallan");
process.exit(failed === 0 ? 0 : 1);
