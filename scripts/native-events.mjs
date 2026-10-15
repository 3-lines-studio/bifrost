// Runs the reader of a stream, which is generated as a string of Go and so is
// executed by no other test. The module is cut out of the generator and fed the
// shapes a server really sends: an event, a comment, several lines of data,
// several events in one piece, a piece that cuts a line in the middle, a piece
// that cuts the blank line between two events, a carriage return, an event named
// with `event:`, an `id:` that has to be kept, and a response with no trailing
// newline.
//
// Run it with `make native-events`.

import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import path from "node:path";

const require = createRequire(import.meta.url);

const source = await readFile(new URL("../cmd/bifrost/native.go", import.meta.url), "utf8");
const marker = "const nativeEventSource = `";
const start = source.indexOf(marker);
if (start < 0) throw new Error("nativeEventSource is not in the generator");
const from = start + marker.length;
const to = source.indexOf("\n`", from);
if (to < 0) throw new Error("nativeEventSource has no end");
const module = source.slice(from, to);
if (module.includes("`")) throw new Error("the module carries a backtick, which is the end of the Go string");

const directory = await mkdtemp(path.join(tmpdir(), "bifrost-events-"));
const file = path.join(directory, "events.ts");
// El original lo escribe el generador, y acá es el de un servidor cualquiera: lo
// que se mide es el lector, no de dónde sale el host.
await writeFile(path.join(directory, "origin.ts"), 'export const ORIGIN = "https://ejemplo.test";\n');
await writeFile(file, module);
const { Messages } = require(file);

// Lo que un lector entrega, y el id que queda puesto.
function read(pieces) {
  const reader = new Messages();
  const out = [];
  for (const piece of pieces) {
    for (const one of reader.push(piece)) {
      out.push([one.data, one.event, one.id]);
    }
  }
  return out;
}

const cases = [
  ["un evento", ["data: hola\n\n"], [["hola", "", null]]],
  ["dos lineas de datos son un evento con salto", ["data: uno\ndata: dos\n\n"], [["uno\ndos", "", null]]],
  ["un comentario no dice nada", [": keep-alive\n\n"], []],
  ["dos eventos en una pieza", ["data: uno\n\ndata: dos\n\n"], [["uno", "", null], ["dos", "", null]]],
  [
    "una pieza que corta la linea al medio",
    ["data: ho", "la\n", "\n"],
    [["hola", "", null]],
  ],
  [
    "una pieza que corta el blanco entre dos eventos",
    ["data: uno\n\ndata: dos\n", "\n"],
    [["uno", "", null], ["dos", "", null]],
  ],
  ["un retorno de carro se come", ["data: hola\r\n\r\n"], [["hola", "", null]]],
  ["el nombre del evento viaja", ["event: live\ndata: {}\n\n"], [["{}", "live", null]]],
  ["el id se guarda y viaja con el evento", ["id: 7\ndata: x\n\n"], [["x", "", "7"]]],
  [
    "el id queda para el que sigue",
    ["id: 7\ndata: x\n\n", "data: y\n\n"],
    [["x", "", "7"], ["y", "", "7"]],
  ],
  ["el id del segundo pisa al del primero", ["id: 1\ndata: x\n\nid: 2\ndata: y\n\n"], [["x", "", "1"], ["y", "", "2"]]],
  ["un espacio solo, despues de los dos puntos, no cuenta", ["data:  dos espacios\n\n"], [[" dos espacios", "", null]]],
  ["un campo sin dos puntos vale vacio", ["data\n\n"], [["", "", null]]],
  [
    "lo que quedo a medias espera a la pieza siguiente",
    ["data: uno\n\ndata: par", "cial"],
    [["uno", "", null]],
  ],
  ["una pieza vacia no entrega nada", ["", "\n"], []],
  [
    "el log real de la app: un evento y lo vivo, una pieza cada uno",
    ['id: 5\ndata: {"text":"hola"}\n\n', 'data: {"event":"live","message":"esc"}\n\n'],
    [['{"text":"hola"}', "", "5"], ['{"event":"live","message":"esc"}', "", "5"]],
  ],
];

let failed = 0;
for (const [name, pieces, wanted] of cases) {
  let got;
  try {
    got = read(pieces);
  } catch (error) {
    failed += 1;
    console.log("FALLA " + name + ": tiró " + error.message);
    continue;
  }
  if (JSON.stringify(got) !== JSON.stringify(wanted)) {
    failed += 1;
    console.log("FALLA " + name);
    console.log("  piezas   " + JSON.stringify(pieces));
    console.log("  esperado " + JSON.stringify(wanted));
    console.log("  obtenido " + JSON.stringify(got));
  }
}

// El EventSource entero: una conexión que se abre, entrega y se cierra, con un
// XMLHttpRequest de mentira que hace de servidor de a una pieza por vez.
const fake = [];
globalThis.XMLHttpRequest = function XMLHttpRequest() {
  const self = this;
  this.readyState = 0;
  this.status = 0;
  this.responseText = "";
  this.onreadystatechange = null;
  this.onerror = null;
  this.sent = null;
  this.open = (method, url) => {
    self.method = method;
    self.url = url;
  };
  this.setRequestHeader = (name, value) => {
    self[name.toLowerCase()] = value;
  };
  this.send = () => {
    self.sent = true;
    globalThis.__sent = self;
  };
  this.abort = () => {
    self.aborted = true;
  };
  fake.push(self);
};
globalThis.setTimeout = () => 0;
globalThis.clearTimeout = () => {};

const { EventSource } = require(file);
const seen = [];
const streamed = new EventSource("/api/stream?project=p&thread=t&since=0");
streamed.addEventListener("message", (event) => seen.push(event.data));
const connection = globalThis.__sent;
// El servidor abre y manda dos eventos, uno por vez. La conexion dice su texto
// entero cada vez, que es lo que una conexion hace: lo que no se leyo todavia es
// lo que agrego.
connection.status = 200;
connection.readyState = 3;
connection.responseText = "id: 1\ndata: uno\n\n";
connection.onreadystatechange();
const afterFirst = seen.slice();
connection.responseText += "id: 2\ndata: dos\n\n";
connection.onreadystatechange();
// Y avisar otra vez, sin que haya nada nuevo, no entrega nada de nuevo.
connection.onreadystatechange();

const checks = [
  ["una url sin host va contra el origen", connection.url === "https://ejemplo.test/api/stream?project=p&thread=t&since=0"],
  ["entrega el primero apenas llega", JSON.stringify(afterFirst) === '["uno"]'],
  ["y el segundo cuando el servidor lo escribe", JSON.stringify(seen) === '["uno","dos"]'],
  ["guarda el ultimo id", streamed.lastEventId === "2"],
  ["quedo abierto", streamed.readyState === 1],
];
for (const [name, ok] of checks) {
  if (!ok) {
    failed += 1;
    console.log("FALLA " + name + " (seen " + JSON.stringify(seen) + ", id " + streamed.lastEventId + ", estado " + streamed.readyState + ")");
  }
}

// Un servidor que contesta 404 no se vuelve a pedir: preguntar otra vez es
// preguntar lo mismo para siempre.
const failing = new EventSource("/api/stream");
const refused = globalThis.__sent;
refused.status = 404;
refused.readyState = 4;
refused.onreadystatechange();
if (failing.readyState !== 2 || failing.asked !== 0) {
  failed += 1;
  console.log("FALLA un 404 se vuelve a pedir");
}

const total = cases.length + checks.length + 1;
console.log(failed === 0 ? total + " casos, todos bien" : failed + " de " + total + " fallan");
process.exit(failed === 0 ? 0 : 1);
