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
  check("21 component instances", (await cards.count()) === 21);
  check("@scope applies", (await cards.first().evaluate((el) => getComputedStyle(el).borderTopWidth)) === "1px");
  check("card template default", (await cards.first().locator(".card__summary").textContent()) === "Normal DOM, Alpine and @scope.");
  check("card inline kicker", (await cards.first().locator(".card__kicker").textContent()) === "Component");
  const inlineCard = cards.filter({ hasText: "Inline card" });
  check("card inline list data", (await inlineCard.locator(".card__tags li").allTextContents()).join(",") === "go,alpine");
  const dropdown = page.locator('div[data-bifrost-component="dropdown"]');
  const menu = dropdown.locator(".dropdown__menu");
  check("dropdown starts closed", !(await menu.isVisible()));
  await dropdown.locator("button").click();
  await page.waitForTimeout(150);
  check("dropdown opens", await menu.isVisible());
  await page.mouse.click(5, 200);
  await page.waitForTimeout(150);
  check("dropdown closes outside", !(await menu.isVisible()));

  await page.waitForFunction(() => document.querySelector('div[data-bifrost-component="counter"]')?.__bifrostMounted === true, null, { timeout: 5000 });
  const counter = page.locator('div[data-bifrost-component="counter"]');
  check("component js starts", (await counter.locator("[data-count]").textContent()) === "0");
  await counter.locator("button").click();
  check("component js mounts", (await counter.locator("[data-count]").textContent()) === "1");

  await page.evaluate(() => window.Turbo.visit("/"));
  await page.waitForURL(`${base}/`);
  await page.waitForFunction(() => document.title === "Bifrost vanilla");
  await page.evaluate(() => window.Turbo.visit("/components-demo"));
  await page.waitForURL(`${base}/components-demo`);
  await page.waitForFunction(() => document.querySelector('div[data-bifrost-component="counter"]')?.__bifrostMounted === true, null, { timeout: 5000 });
  const counterAgain = page.locator('div[data-bifrost-component="counter"]');
  check("component js remounts", (await counterAgain.locator("[data-count]").textContent()) === "0");
  await counterAgain.locator("button").click();
  check("component js has one listener", (await counterAgain.locator("[data-count]").textContent()) === "1");

  for (const error of errors) console.log(`  page error: ${error}`);
  check("no page errors", errors.length === 0);
  console.log("all browser checks passed");
} finally {
  await browser.close();
}
