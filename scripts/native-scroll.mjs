// Runs the decision of what a block that scrolls is, which is generated as a
// string of Go and so is executed by no other test. The module is cut out of the
// generator and fed the classes a stylesheet really has: the one that scrolls
// down, the one that scrolls across, the one that asks for both, the one that
// only clips, the ones with variants, what is not a class at all, the class of a
// child the scroll view has to pin, and the class of a box pinned to the screen.
//
// Run it with `make native-scroll`.

import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import path from "node:path";

const require = createRequire(import.meta.url);

const source = await readFile(new URL("../cmd/bifrost/native.go", import.meta.url), "utf8");
const marker = "const nativeScroll = `";
const start = source.indexOf(marker);
if (start < 0) throw new Error("nativeScroll is not in the generator");
const from = start + marker.length;
const to = source.indexOf("\n`", from);
if (to < 0) throw new Error("nativeScroll has no end");
const module = source.slice(from, to);
if (module.includes("`")) throw new Error("the module carries a backtick, which is the end of the Go string");
if (module.includes("\\\\s")) throw new Error("a regex of the module carries an escaped backslash");

const directory = await mkdtemp(path.join(tmpdir(), "bifrost-scroll-"));
const file = path.join(directory, "scroll.js");
await writeFile(file, module);
const { isBottomAnchored, isFixed, isSticky, scrollOf } = require(file);

const cases = [
  ["nada que scrollee", "flex-1 min-h-0", null],
  ["scrollea abajo", "min-h-0 flex-1 overflow-y-auto", "vertical"],
  ["scrollea abajo, con scroll", "overflow-y-scroll", "vertical"],
  ["el generico scrollea abajo", "overflow-auto", "vertical"],
  ["scrollea al costado", "flex overflow-x-auto", "horizontal"],
  ["los dos: gana el de abajo, que es el que alcanza lo que quedo abajo", "overflow-x-auto overflow-y-auto", "vertical"],
  ["el que solo recorta no scrollea: lo expresa el compilador", "overflow-hidden", null],
  ["el que recorta al costado tampoco", "overflow-x-hidden", null],
  ["una clase con variante no es la clase", "md:overflow-y-auto", null],
  ["una clase parecida no alcanza", "overflowy-auto", null],
  ["el nombre solo, entre otras clases", "relative flex-1 rounded-lg overflow-y-auto p-2", "vertical"],
  ["con espacios de mas", "   overflow-y-auto   flex-1  ", "vertical"],
  ["una cadena vacia no scrollea", "", null],
  ["lo que no es cadena no scrollea", null, null],
  ["un numero no scrollea", 7, null],
];

const stickyCases = [
  ["la clase de un hijo que la vista que scrollea tiene que clavar", "sticky top-0 z-40 bg-white", true],
  ["el nombre solo", "sticky", true],
  ["una clase con variante no es la clase", "md:sticky", false],
  ["una clase parecida no alcanza", "stickyy", false],
  ["no esta", "relative top-0", false],
  ["una cadena vacia no se clava", "", false],
  ["lo que no es cadena no se clava", null, false],
];

let failed = 0;
for (const [name, className, wanted] of cases) {
  let got;
  try {
    got = scrollOf(className);
  } catch (error) {
    failed += 1;
    console.log("FALLA " + name + ": tiró " + error.message);
    continue;
  }
  if (got !== wanted) {
    failed += 1;
    console.log("FALLA " + name);
    console.log("  entrada  " + JSON.stringify(className));
    console.log("  esperado " + JSON.stringify(wanted));
    console.log("  obtenido " + JSON.stringify(got));
  }
}

for (const [name, className, wanted] of stickyCases) {
  let got;
  try {
    got = isSticky(className);
  } catch (error) {
    failed += 1;
    console.log("FALLA " + name + ": tiró " + error.message);
    continue;
  }
  if (got !== wanted) {
    failed += 1;
    console.log("FALLA " + name);
    console.log("  entrada  " + JSON.stringify(className));
    console.log("  esperado " + JSON.stringify(wanted));
    console.log("  obtenido " + JSON.stringify(got));
  }
}

const fixedCases = [
  ["una caja clavada arriba", "fixed left-0 right-0 top-0 z-50", true, false],
  ["una caja clavada abajo", "fixed bottom-0 left-0 right-0 z-50", true, true],
  ["abajo pero sin clavarla", "absolute bottom-0", false, true],
  ["una variante no es la clase", "md:fixed bottom-0", false, true],
  ["una clase parecida no alcanza", "fixed-bottom", false, false],
];

for (const [name, className, fixed, bottom] of fixedCases) {
  let gotFixed;
  let gotBottom;
  try {
    gotFixed = isFixed(className);
    gotBottom = isBottomAnchored(className);
  } catch (error) {
    failed += 1;
    console.log("FALLA " + name + ": tiró " + error.message);
    continue;
  }
  if (gotFixed !== fixed || gotBottom !== bottom) {
    failed += 1;
    console.log("FALLA " + name);
    console.log("  entrada  " + JSON.stringify(className));
    console.log("  esperado " + JSON.stringify([fixed, bottom]));
    console.log("  obtenido " + JSON.stringify([gotFixed, gotBottom]));
  }
}

const total = cases.length + stickyCases.length + fixedCases.length;
console.log(failed === 0 ? total + " casos, todos bien" : failed + " de " + total + " fallan");
process.exit(failed === 0 ? 0 : 1);
