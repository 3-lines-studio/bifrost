import assert from "node:assert/strict";
import { chromium } from "playwright-core";

const base = "http://127.0.0.1:18801";
const browser = await chromium.launch();
const page = await browser.newPage();
const errors = [];
page.on("pageerror", (error) => errors.push(error.message));

const check = (name, condition) => {
  assert.ok(condition, name);
  console.log(`  ok   ${name}`);
};

try {
  await page.goto(`${base}/`, { waitUntil: "networkidle" });
  await page.waitForFunction(() => /\d{4}-\d\d-\d\dT/.test(document.querySelector("section.panel strong")?.textContent || ""), null, { timeout: 5000 });
  check("home title", (await page.title()) === "Bifrost vanilla");
  check("Go API feeds Alpine", (await page.locator("section.panel strong").first().textContent()).includes("T"));
  check("boot tag", (await page.locator("#boot").textContent()) === "Turbo + Alpine");

  await page.evaluate(() => { window.__marker = "alive"; });
  await page.click('a[href="/posts"]');
  await page.waitForURL("**/posts");
  check("posts title", (await page.title()).startsWith("Posts"));
  check("Turbo kept the JS context", (await page.evaluate(() => window.__marker)) === "alive");
  check("active nav", (await page.locator('a[aria-current="page"]').getAttribute("href")) === "/posts");

  await page.click('a[href="/posts/hello"]');
  await page.waitForURL("**/posts/hello");
  check("post title", (await page.title()).startsWith("Hello, vanilla"));
  check("post params", (await page.locator("h1").textContent()) === "Hello, vanilla");

  await page.goBack();
  await page.waitForURL("**/posts");
  await page.goForward();
  await page.waitForURL("**/posts/hello");
  check("history", (await page.locator("h1").textContent()) === "Hello, vanilla");

  await page.evaluate(() => window.Turbo.visit("/nope"));
  await page.waitForFunction(() => document.querySelector("h1")?.textContent === "Not found");
  check("404 via Turbo", (await page.evaluate(() => window.__marker)) === "alive");

  await page.evaluate(() => window.Turbo.visit("/posts/boom"));
  await page.waitForFunction(() => document.querySelector("h1")?.textContent === "Posts error");
  check("nearest error view", (await page.evaluate(() => window.__marker)) === "alive");

  await page.evaluate(() => window.Turbo.visit("/posts/missing"));
  await page.waitForFunction(() => document.querySelector("h1")?.textContent === "Not found");
  check("loader not found", (await page.evaluate(() => window.__marker)) === "alive");

  await page.goto(`${base}/`, { waitUntil: "networkidle" });
  await page.evaluate(() => { window.__marker = "alive"; });
  await page.click('a[href="/island"]');
  await page.waitForURL("**/island");
  check("island full load", (await page.evaluate(() => window.__marker)) === undefined);
  await page.click("#island-button");
  check("page.ts island", /^\d\d:\d\d:\d\d$/.test(await page.locator("#island-output").textContent()));

  await page.goto(`${base}/`, { waitUntil: "networkidle" });
  await page.evaluate(() => { window.__marker = "alive"; });
  await page.click('a[href="/components-demo"]');
  await page.waitForURL("**/components-demo");
  check("components via Turbo", (await page.evaluate(() => window.__marker)) === "alive");
  const cards = page.locator('div[data-bifrost-component="card"]');
  check("20 component instances", (await cards.count()) === 20);
  check("@scope applies", (await cards.first().evaluate((el) => getComputedStyle(el).borderTopWidth)) === "1px");
  const dropdown = page.locator('div[data-bifrost-component="dropdown"]');
  const menu = dropdown.locator(".dropdown__menu");
  check("dropdown starts closed", !(await menu.isVisible()));
  await dropdown.locator("button").click();
  await page.waitForTimeout(150);
  check("dropdown opens", await menu.isVisible());
  await page.mouse.click(5, 200);
  await page.waitForTimeout(150);
  check("dropdown closes outside", !(await menu.isVisible()));

  check("no page errors", errors.length === 0);
  console.log("all browser checks passed");
} finally {
  await browser.close();
}
