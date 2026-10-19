// Runs the decision of what a block that scrolls is, which is generated as a
// string of Go and so is executed by no other test. The module is cut out of the
// generator and fed the classes a stylesheet really has: the one that scrolls
// down, the one that scrolls across, the one that asks for both, the one that
// only clips, the ones with variants, what is not a class at all, and the class
// of a child the scroll view has to pin.
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
const { isSticky, scrollOf } = require(file);

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

const total = cases.length + stickyCases.length;
console.log(failed === 0 ? total + " casos, todos bien" : failed + " de " + total + " fallan");
process.exit(failed === 0 ? 0 : 1);
