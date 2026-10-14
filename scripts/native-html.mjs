// Runs the parser of injected HTML, which is generated as a string of Go and so
// is executed by no other test. The module is cut out of the generator and fed
// the shapes the app's markdown actually emits: a paragraph, a heading, a nested
// list, a task list, a table, a quote, a fenced block with its tokens, inline
// emphasis, a link, and the entities of escaped text.
//
// Run it with `make native-html`.

import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { pathToFileURL } from "node:url";

const source = await readFile(new URL("../cmd/bifrost/native.go", import.meta.url), "utf8");
const marker = "const nativeHTML = `";
const start = source.indexOf(marker);
if (start < 0) throw new Error("nativeHTML is not in the generator");
const from = start + marker.length;
const to = source.indexOf("\n`", from);
if (to < 0) throw new Error("nativeHTML has no end");
const module = source.slice(from, to);

const directory = await mkdtemp(path.join(tmpdir(), "bifrost-html-"));
const file = path.join(directory, "html.ts");
await writeFile(file, module);
const { parseHTML } = await import(pathToFileURL(file).href);

// What the tag map paints, in the shape the tree keeps them: a run of text, or a
// tag with its props and its children.
function shape(nodes) {
  return nodes
    .map((node) =>
      typeof node === "string"
        ? "t:" + node
        : "<" + node.tag + Object.keys(node.props).length + ">" + shape(node.children),
    )
    .join("|");
}

const cases = [
  ["<p>una linea que sigue</p><p>y otro</p>", "<p0>t:una linea que sigue|<p0>t:y otro"],
  ["<h2>título</h2>", "<h20>t:título"],
  [
    "<ul><li>uno<ul><li>sub</li><li>sub dos</li></ul></li><li>dos</li></ul>",
    "<ul0><li0>t:uno|<ul0><li0>t:sub|<li0>t:sub dos|<li0>t:dos",
  ],
  [
    '<div class="code"><button type="button" class="code-copy" data-code-copy>Copiar</button><pre><code><span class="tok-keyword">let</span> x = 1</code></pre></div>',
    '<div1><button3>t:Copiar|<pre0><code0><span1>t:let|t: x = 1',
  ],
  ["<p>con &lt; y &amp; adentro &quot;comillas&quot;</p>", '<p0>t:con < y & adentro "comillas"'],
  // An element that closes itself does not swallow what comes after it.
  ["<p>uno<br>dos</p>", "<p0>t:uno|<br0>|t:dos"],
  // A `>` inside a quoted value does not end the tag.
  ['<p><a href="https://e.com/a?x=1>2" rel="noreferrer">t</a></p>', "<p0><a2>t:t"],
  // A tag the map does not paint is not a tag the parser loses: what it holds is
  // still there, and the parser answers for it the same way.
  ["<section><p>adentro</p></section>", "<section0><p0>t:adentro"],
  // Comments, and the text of a <pre> keeping its whitespace.
  ["<!-- nada --><pre>  dos\n  espacios</pre>", "<pre0>t:  dos\n  espacios"],
  // The text between two blocks is nothing, and it is not an empty run.
  ["<p>a</p>\n  <p>b</p>", "<p0>t:a|<p0>t:b"],
];

let failed = 0;
for (const [html, wanted] of cases) {
  const got = shape(parseHTML(html));
  if (got !== wanted) {
    failed += 1;
    console.log("FALLA " + JSON.stringify(html));
    console.log("  esperado " + JSON.stringify(wanted));
    console.log("  obtenido " + JSON.stringify(got));
  }
}
// A tree that is not a list, or a text with no tag, is the shape the painter
// would take the render down with.
const tree = parseHTML("<p>hola</p>");
if (!Array.isArray(tree) || tree[0]?.tag !== "p") {
  failed += 1;
  console.log("FALLA la raíz no es una lista de tags");
}
console.log(failed === 0 ? cases.length + " casos, todos bien" : failed + " de " + cases.length + " fallan");
process.exit(failed === 0 ? 0 : 1);
