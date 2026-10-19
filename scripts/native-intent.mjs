// Runs the decision of what a url that is not a page of the app is, which is
// generated as a string of Go and so is executed by no other test. The module is
// cut out of the generator and fed the urls a link and a server really hand over:
// the one a mail opens the app with, the one a redirect answers with, the one the
// api is asked for, and the ones that leave this origin.
//
// The request itself is faked, because what is being tested is the decision and
// not the network: each case says what the server answers, and the test says what
// was asked for and where the app ends up.
//
// Run it with `make native-intent`.

import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import path from "node:path";

const require = createRequire(import.meta.url);

const ORIGIN = "http://10.0.2.2:8099";

const source = await readFile(new URL("../cmd/bifrost/native.go", import.meta.url), "utf8");
const marker = "const nativeIntent = `";
const start = source.indexOf(marker);
if (start < 0) throw new Error("nativeIntent is not in the generator");
const from = start + marker.length;
const to = source.indexOf("\n`", from);
if (to < 0) throw new Error("nativeIntent has no end");
const module = source.slice(from, to);
if (module.includes("`")) throw new Error("the module carries a backtick, which is the end of the Go string");
if (module.includes("\\\\s")) throw new Error("a regex of the module carries an escaped backslash");

const directory = await mkdtemp(path.join(tmpdir(), "bifrost-intent-"));
await writeFile(path.join(directory, "origin.ts"), `export const ORIGIN = ${JSON.stringify(ORIGIN)};\n`);
await writeFile(path.join(directory, "intent.ts"), module);
const { follow, pathOf } = require(path.join(directory, "intent.ts"));

// What the server answers, per case: a status and, when it redirects, where to.
const answers = new Map();
let asked = [];
globalThis.fetch = async (url, init) => {
  asked.push([url, init?.redirect]);
  const answer = answers.get(url);
  if (answer === undefined) {
    throw new Error("nadie dijo que el server conteste " + url);
  }
  return {
    status: answer.status,
    headers: { get: (name) => (name.toLowerCase() === "location" ? answer.location ?? null : null) },
  };
};

const paths = [
  ["el url entero de un deep link de desarrollo", `exp://10.0.2.2:8081/--/auth?token=a1`, "/auth?token=a1"],
  ["el de produccion, que ya trae el dominio", "https://fonnso.com/auth?token=a1", "/auth?token=a1"],
  ["el que abre la app en su raiz", "exp://10.0.2.2:8081", "/"],
  ["el de la raiz con la barra", "exp://10.0.2.2:8081/", "/"],
  ["una direccion que ya es un path", "/onboarding?paso=2", "/onboarding?paso=2"],
  ["una direccion pelada, sin barra", "onboarding", "/onboarding"],
  ["un fragmento no es parte de lo que se pide", "/thread#abajo", "/thread#abajo"],
];

let failed = 0;
const falla = (name, said, wanted, got) => {
  failed += 1;
  console.log("FALLA " + name);
  console.log("  " + said + " " + JSON.stringify(wanted));
  console.log("  obtenido " + JSON.stringify(got));
};

for (const [name, url, wanted] of paths) {
  const got = pathOf(url);
  if (got !== wanted) falla(name, "esperado", wanted, got);
}

const cases = [
  {
    name: "un link de un mail: la cookie entra y la app va a donde el server manda",
    address: "/auth?token=a1",
    answer: { status: 303, location: "/" },
    wanted: { kind: "inside", href: "/" },
    asked: ["http://10.0.2.2:8099/auth?token=a1", "manual"],
  },
  {
    name: "un redirect absoluto del propio origen",
    address: "/auth?token=a1",
    answer: { status: 303, location: ORIGIN + "/onboarding" },
    wanted: { kind: "inside", href: "/onboarding" },
    asked: ["http://10.0.2.2:8099/auth?token=a1", "manual"],
  },
  {
    name: "un redirect que se va del origen: va al navegador",
    address: "/auth?token=a1",
    answer: { status: 303, location: "https://checkout.stripe.com/c/pay/x" },
    wanted: { kind: "outside", href: "https://checkout.stripe.com/c/pay/x" },
    asked: ["http://10.0.2.2:8099/auth?token=a1", "manual"],
  },
  {
    name: "un 200 es un documento del server, y este target no lo sabe pintar",
    address: "/preview/abc/web/",
    answer: { status: 200 },
    wanted: null,
    asked: ["http://10.0.2.2:8099/preview/abc/web/", "manual"],
  },
  {
    name: "un 303 sin location no dice a donde ir",
    address: "/auth?token=a1",
    answer: { status: 303 },
    wanted: null,
    asked: ["http://10.0.2.2:8099/auth?token=a1", "manual"],
  },
  {
    // The dance of an oauth provider starts by asking this url, and the cookie
    // of the state belongs to whoever asked: a browser that asks is a browser
    // that ends the dance with a session this app will never have.
    name: "el link de google se pide, y la cookie del state queda en la app",
    address: "/api/auth/google",
    answer: { status: 303, location: "/" },
    wanted: { kind: "inside", href: "/" },
    asked: ["http://10.0.2.2:8099/api/auth/google", "manual"],
  },
  {
    // The request creates the checkout session, so it is the app that makes it.
    name: "el de la api de pago tambien se pide: el pedido crea la sesion",
    address: "/api/stripe/checkout?plan=pro",
    answer: { status: 303, location: "/" },
    wanted: { kind: "inside", href: "/" },
    asked: ["http://10.0.2.2:8099/api/stripe/checkout?plan=pro", "manual"],
  },
  {
    name: "pedir la api no obliga a quedarse: el redirect que sale va al navegador",
    address: "/api/stripe/checkout?plan=pro",
    answer: { status: 303, location: "https://checkout.stripe.com/x" },
    wanted: { kind: "outside", href: "https://checkout.stripe.com/x" },
    asked: ["http://10.0.2.2:8099/api/stripe/checkout?plan=pro", "manual"],
  },
  {
    name: "un url de otro origen no es de este servidor",
    address: "https://otro.com/auth?token=a1",
    answer: { status: 303, location: "/" },
    wanted: null,
    asked: [],
  },
  {
    name: "uno que parece relativo y se va del origen tampoco",
    address: "//otro.com/auth",
    answer: { status: 303, location: "/" },
    wanted: null,
    asked: [],
  },
  {
    name: "un url del propio servidor se pide como path",
    address: "http://10.0.2.2:8099/auth?token=a1",
    answer: { status: 303, location: "/" },
    wanted: { kind: "inside", href: "/" },
    asked: ["http://10.0.2.2:8099/auth?token=a1", "manual"],
  },
];

// The answer is a value of two fields, so a comparison by identity would say a
// right answer is wrong.
const sameAnswer = (got, wanted) =>
  wanted === null ? got === null : got !== null && got.kind === wanted.kind && got.href === wanted.href;

for (const one of cases) {
  asked = [];
  answers.clear();
  const url = one.asked[0] ?? ORIGIN + one.address;
  answers.set(url, one.answer);
  let got;
  try {
    got = await follow(one.address);
  } catch (error) {
    falla(one.name, "no tenia que tirar:", error.message, "throw");
    continue;
  }
  if (!sameAnswer(got, one.wanted)) falla(one.name, "esperado", one.wanted, got);
  const expectedAsked = one.asked.length === 0 ? [] : [one.asked];
  if (JSON.stringify(asked) !== JSON.stringify(expectedAsked)) {
    falla(one.name + " (los pedidos)", "esperado", expectedAsked, asked);
  }
}

console.log(failed === 0 ? cases.length + " respuestas y " + paths.length + " paths, todos bien" : failed + " fallan");
process.exit(failed === 0 ? 0 : 1);
