import { readFile, writeFile } from "node:fs/promises";
import { chromium } from "playwright-core";

const sourcePath = new URL("../example/basic/pages/home.tsx", import.meta.url);
const staticPath = new URL("../example/basic/pages/about.tsx", import.meta.url);
const clientPath = new URL("../example/basic/pages/app.tsx", import.meta.url);
const original = await readFile(sourcePath, "utf8");
const originalStatic = await readFile(staticPath, "utf8");
const originalClient = await readFile(clientPath, "utf8");
const browser = await chromium.launch({
  executablePath: process.env.CHROMIUM_PATH || "/usr/bin/chromium",
  headless: true,
});
try {
  const page = await browser.newPage();
  await page.goto("http://127.0.0.1:8080/?name=Dev", { waitUntil: "networkidle" });
  if ((await page.locator("h1").textContent()) !== "Hello Dev") throw new Error("unexpected initial development page");

  const changed = original.replace("<h1>Hello ", "<h1>Welcome ");
  if (changed === original) throw new Error("development fixture replacement did not match");
  await writeFile(sourcePath, changed);
  await page.waitForFunction(() => document.querySelector("h1")?.textContent === "Welcome Dev", undefined, { timeout: 60_000 });
  await page.reload({ waitUntil: "networkidle" });
  if ((await page.locator("h1").textContent()) !== "Welcome Dev") {
    throw new Error("Vite SSR module graph did not update");
  }

  await writeFile(sourcePath, `${changed}\nexport const syntaxError = ;\n`);
  await page.locator("vite-error-overlay").waitFor({ state: "attached", timeout: 60_000 });
  await new Promise((resolve) => setTimeout(resolve, 100));
  await writeFile(sourcePath, changed);
  await page.locator("vite-error-overlay").waitFor({ state: "detached", timeout: 60_000 });
  await page.waitForFunction(() => document.querySelector("h1")?.textContent === "Welcome Dev", undefined, { timeout: 60_000 });

  await page.goto("http://127.0.0.1:8080/about", { waitUntil: "networkidle" });
  const changedStatic = originalStatic.replace("<h1>About", "<h1>Static HMR");
  if (changedStatic === originalStatic) throw new Error("static fixture replacement did not match");
  await writeFile(staticPath, changedStatic);
  await page.waitForFunction(() => document.querySelector("h1")?.textContent === "Static HMR", undefined, { timeout: 60_000 });
  await page.reload({ waitUntil: "networkidle" });
  if ((await page.locator("h1").textContent()) !== "Static HMR") {
    throw new Error("Static development SSR did not use Vite's live module graph");
  }

  await page.goto("http://127.0.0.1:8080/app", { waitUntil: "domcontentloaded" });
  const button = page.locator("button");
  await button.waitFor();
  await button.click();
  if ((await button.textContent()) !== "Count 1") throw new Error("client state setup failed");
  const changedClient = originalClient.replace("Count {count}", "Clicks {count}");
  if (changedClient === originalClient) throw new Error("client fixture replacement did not match");
  await writeFile(clientPath, changedClient);
  await page.waitForFunction(() => document.querySelector("button")?.textContent === "Clicks 1", undefined, { timeout: 60_000 });
} finally {
  await writeFile(sourcePath, original);
  await writeFile(staticPath, originalStatic);
  await writeFile(clientPath, originalClient);
  await browser.close();
}
console.log("development browser reload passed");
