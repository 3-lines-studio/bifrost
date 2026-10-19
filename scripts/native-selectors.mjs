// Runs the rewrite that turns an element selector into the class the tag map
// paints an element with, which is generated as a string of Go and so is
// executed by no other test. The module is cut out of the generator and run over
// the selectors a stylesheet really has: the one that names a tag, a list, a
// descendant, the :where() the app of the case writes, a pseudo-element, a
// position in the tree, a combinator, an animation, and a rule that names no
// tag at all.
//
// Run it with `make native-selectors`.

import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import path from "node:path";

const require = createRequire(import.meta.url);

const source = await readFile(new URL("../cmd/bifrost/native.go", import.meta.url), "utf8");
const marker = "const nativeSelectors = `";
const start = source.indexOf(marker);
if (start < 0) throw new Error("nativeSelectors is not in the generator");
const from = start + marker.length;
const to = source.indexOf("\n`", from);
if (to < 0) throw new Error("nativeSelectors has no end");
const module = source.slice(from, to);

const directory = await mkdtemp(path.join(tmpdir(), "bifrost-selectors-"));
const file = path.join(directory, "selectors.js");
await writeFile(file, module);
const { withTagClasses } = require(file);

// The whitespace of a stylesheet is not what this checks: the parser writes the
// selectors back with its own, and the compiler reads either.
const norm = (css) => css.replace(/\s+/g, " ").replace(/\s*,\s*/g, ", ").trim();

function run(css) {
  const lost = [];
  const out = withTagClasses(css, (selector, what) => lost.push([selector, what]));
  return { out: norm(out), lost };
}

const cases = [
  [
    "una regla que nombra un tag",
    ".md p { margin: 0 }",
    ({ out }) => out === ".md .native-p { margin: 0 }",
  ],
  [
    "un tag solo, sin clase",
    "h2 { font-size: 1rem }",
    ({ out }) => out === ".native-h2 { font-size: 1rem }",
  ],
  [
    "una lista de selectores, los dos",
    ".md ul, .md ol { padding-left: 1.25rem }",
    ({ out }) => out === ".md .native-ul, .md .native-ol { padding-left: 1.25rem }",
  ],
  [
    "el :where que escribe la app se abre en sus alternativas",
    ".md :where(h2, h3, h4) { margin: 1rem 0 0.5rem }",
    ({ out }) => out === ".md .native-h2, .md .native-h3, .md .native-h4 { margin: 1rem 0 0.5rem }",
  ],
  [
    "un :where con una clase adentro, cada alternativa entera",
    ".md :where(p, blockquote, .table-wrap) { margin: 0 0 0.75rem }",
    ({ out }) =>
      out === ".md .native-p, .md .native-blockquote, .md .table-wrap { margin: 0 0 0.75rem }",
  ],
  [
    "dos tags en un mismo compuesto",
    "pre code { padding: 0 }",
    ({ out }) => out === ".native-pre .native-code { padding: 0 }",
  ],
  [
    "un hijo directo de un tag",
    "table > tr { border: 0 }",
    ({ out }) => out === ".native-table > .native-tr { border: 0 }",
  ],
  [
    "un atributo no se toca",
    'input[type="email"] { color: red }',
    ({ out }) => out === '.native-input[type="email"] { color: red }',
  ],
  [
    "un pseudo-elemento se avisa, y la regla queda para el browser",
    ".md li::marker { color: gray }",
    ({ out, lost }) =>
      out === ".md .native-li::marker { color: gray }" &&
      lost.length === 1 &&
      lost[0][0] === ".md li::marker" &&
      lost[0][1] === "::marker",
  ],
  [
    "una posicion en el arbol se avisa",
    ".md > :first-child { margin-top: 0 }",
    ({ out, lost }) =>
      out === ".md > :first-child { margin-top: 0 }" &&
      lost.length === 1 &&
      lost[0][1] === ":first-child",
  ],
  [
    "la raiz del documento no se avisa: el compilador la expresa",
    ":root { --x: 1 }",
    ({ out, lost }) => out === ":root { --x: 1 }" && lost.length === 0,
  ],
  [
    "un hover no se avisa: el compilador lo expresa",
    ".md .code-copy:hover { opacity: 1 }",
    ({ out, lost }) => out === ".md .code-copy:hover { opacity: 1 }" && lost.length === 0,
  ],
  [
    "un :not() con un tag adentro, el tag cambia igual",
    ".md :not(p) { margin: 0 }",
    ({ out }) => out === ".md :not(.native-p) { margin: 0 }",
  ],
  [
    "un :is() no se abre, y sus tags cambian igual: se avisa",
    ".md :is(p, ul) { margin: 0 }",
    ({ out, lost }) =>
      out === ".md :is(.native-p, .native-ul) { margin: 0 }" &&
      lost.length === 1 &&
      lost[0][0] === ".md :is(p, ul)" &&
      lost[0][1] === ":is(...)",
  ],
  [
    "los pasos de una animacion no son selectores",
    "@keyframes shimmer { from { opacity: 0 } to { opacity: 1 } }",
    ({ out, lost }) =>
      out === "@keyframes shimmer { from { opacity: 0 } to { opacity: 1 } }" && lost.length === 0,
  ],
  [
    "un theme de Tailwind no es una regla",
    "@theme inline { --color-ok: var(--ok) }",
    ({ out, lost }) => out === "@theme inline { --color-ok: var(--ok) }" && lost.length === 0,
  ],
  [
    "una utilidad de Tailwind sin tags vuelve igual",
    "@utility flex { display: flex; flex-direction: row }",
    ({ out, lost }) => out === "@utility flex { display: flex; flex-direction: row }" && lost.length === 0,
  ],
  [
    "adentro de una media query tambien",
    "@media (min-width: 768px) { .md table { font-size: 12px } }",
    ({ out }) => out === "@media (min-width: 768px) { .md .native-table { font-size: 12px } }",
  ],
  [
    "el preflight, que es lo que el pipeline genera",
    "html, :host { font-family: var(--default-font-family) }",
    ({ out }) => out === ".native-html, :host { font-family: var(--default-font-family) }",
  ],
  [
    "el reset que nombra un tag llega al elemento",
    "p, h1 { margin: 0 }",
    ({ out }) => out === ".native-p, .native-h1 { margin: 0 }",
  ],
  [
    "body tambien es una clase",
    "body { line-height: inherit }",
    ({ out }) => out === ".native-body { line-height: inherit }",
  ],
  [
    "una regla sin tags vuelve igual",
    ".flex-1 { flex: 1 1 auto }",
    ({ out, lost }) => out === ".flex-1 { flex: 1 1 auto }" && lost.length === 0,
  ],
  [
    "correrlo dos veces es correrlo una",
    ".md p { margin: 0 }",
    ({ out }) => norm(withTagClasses(out, () => {})) === ".md .native-p { margin: 0 }",
  ],
];

let failed = 0;
for (const [name, input, check] of cases) {
  let result;
  try {
    result = run(input);
  } catch (error) {
    failed += 1;
    console.log("FALLA " + name + ": tiró " + error.message);
    continue;
  }
  let ok;
  try {
    ok = check(result);
  } catch (error) {
    failed += 1;
    console.log("FALLA " + name + ": " + error.message);
    continue;
  }
  if (!ok) {
    failed += 1;
    console.log("FALLA " + name);
    console.log("  entrada  " + JSON.stringify(input));
    console.log("  obtenido " + JSON.stringify(result.out));
    console.log("  avisó    " + JSON.stringify(result.lost));
  }
}
console.log(failed === 0 ? cases.length + " casos, todos bien" : failed + " de " + cases.length + " fallan");
process.exit(failed === 0 ? 0 : 1);
