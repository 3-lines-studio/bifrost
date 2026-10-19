// Runs the resolver that reads a TypeScript file behind the extension NodeNext
// puts in the import, which is generated as a string of Go and so is executed by
// no other test. The module is cut out of the generator and its two functions are
// fed the names a page really has: an import with .js, one with the extension it
// actually resolves to, one reached through the app's alias, and one that is not
// there at all.
//
// Run it with `make native-resolve`.

import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import path from "node:path";

const require = createRequire(import.meta.url);

const source = await readFile(new URL("../cmd/bifrost/native.go", import.meta.url), "utf8");
const marker = "const nativeMetroHelper = `";
const start = source.indexOf(marker);
if (start < 0) throw new Error("nativeMetroHelper is not in the generator");
const from = start + marker.length;
const to = source.indexOf("\n`", from);
if (to < 0) throw new Error("nativeMetroHelper has no end");
const module = source.slice(from, to);

const directory = await mkdtemp(path.join(tmpdir(), "bifrost-resolve-"));
const file = path.join(directory, "metro.js");
await writeFile(file, module);
const { resolveWithTypescript, typescriptOf } = require(file);

let failed = 0;

const cases = [
  ["un import de NodeNext", "./dayjs.js", ["./dayjs.ts", "./dayjs.tsx"]],
  ["el que ya trae la extension real", "./dayjs.ts", []],
  ["una clase de componente", "./button.js", ["./button.ts", "./button.tsx"]],
  ["el alias de la app", "@/lib/util/dayjs.js", ["@/lib/util/dayjs.ts", "@/lib/util/dayjs.tsx"]],
  ["lo que no es un .js", "./style.css", []],
  ["un nombre de paquete no se toca", "dayjs", []],
];

for (const [name, input, wanted] of cases) {
  let got;
  try {
    got = typescriptOf(input);
  } catch (error) {
    failed += 1;
    console.log("FALLA " + name + ": tiró " + error.message);
    continue;
  }
  if (JSON.stringify(got) !== JSON.stringify(wanted)) {
    failed += 1;
    console.log("FALLA " + name);
    console.log("  entrada  " + JSON.stringify(input));
    console.log("  esperado " + JSON.stringify(wanted));
    console.log("  obtenido " + JSON.stringify(got));
  }
}

const resolverCases = [
  ["el archivo existe y se devuelve tal cual", "./dayjs.js", ["./dayjs.js"], "./dayjs.js"],
  ["el .js que no esta se lee como .ts", "./dayjs.js", ["./dayjs.ts"], "./dayjs.ts"],
  ["si no hay .ts, se prueba .tsx", "./button.js", ["./button.tsx"], "./button.tsx"],
  ["si no hay ninguno, se dice el error del .js", "./missing.js", [], null],
  ["lo que no es un .js no se reintenta", "./style.css", [], null],
];

for (const [name, moduleName, available, wanted] of resolverCases) {
  const seen = [];
  const inherited = (context, name, platform) => {
    seen.push(name);
    if (available.includes(name)) {
      return { type: "sourceFile", filePath: name };
    }
    throw new Error("Unable to resolve " + name);
  };
  let got;
  try {
    got = resolveWithTypescript(inherited, {}, moduleName, "android").filePath;
  } catch {
    got = null;
  }
  if (got !== wanted) {
    failed += 1;
    console.log("FALLA " + name);
    console.log("  entrada  " + JSON.stringify(moduleName));
    console.log("  esperado " + JSON.stringify(wanted));
    console.log("  obtenido " + JSON.stringify(got));
    console.log("  preguntó " + JSON.stringify(seen));
  }
}

const total = cases.length + resolverCases.length;
console.log(failed === 0 ? total + " casos, todos bien" : failed + " de " + total + " fallan");
process.exit(failed === 0 ? 0 : 1);
