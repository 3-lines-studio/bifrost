import { chromium } from "playwright-core";

const executablePath = process.env.CHROMIUM_PATH || "/usr/bin/chromium";
const browser = await chromium.launch({ executablePath, headless: true });
try {
  const page = await browser.newPage();
  const browserErrors = [];
  page.on("pageerror", (error) => browserErrors.push(`page error: ${error.message}`));
  page.on("console", (message) => {
    const location = message.location().url;
    if (message.type() === "error" && !location.endsWith("/favicon.ico")) {
      browserErrors.push(`console error: ${message.text()} (${location})`);
    }
  });
  page.on("response", (response) => {
    if (response.status() >= 400 && !response.url().endsWith("/favicon.ico")) {
      browserErrors.push(`${response.status()} ${response.url()}`);
    }
  });
  const assertNoBrowserErrors = () => {
    if (browserErrors.length > 0) throw new Error(browserErrors.join("\n"));
  };
  const assertImageAsset = async () => {
    const image = page.locator("img");
    await image.waitFor();
    const source = await image.getAttribute("src");
    if (!(await image.evaluate((node) => node.complete && node.naturalWidth > 0))) {
      throw new Error(`image asset did not load: ${source}`);
    }
    if (!source?.startsWith("/_bifrost/dist/assets/")) {
      throw new Error(`image uses the wrong asset root: ${source}`);
    }
  };

  await page.goto("http://127.0.0.1:8080/?name=Browser", { waitUntil: "networkidle" });
  if ((await page.locator("h1").textContent()) !== "Hello Browser") {
    throw new Error("server page did not render or hydrate");
  }
  const documentAttributes = await page.locator("html").evaluate((node) => ({ lang: node.lang, className: node.className, dir: node.dir }));
  if (documentAttributes.lang !== "es" || documentAttributes.className !== "theme-dark" || documentAttributes.dir !== "ltr") {
    throw new Error(`request document attributes were lost: ${JSON.stringify(documentAttributes)}`);
  }
  await assertImageAsset();

  await page.goto("http://127.0.0.1:8080/stream", { waitUntil: "networkidle" });
  await page.locator("strong").waitFor();
  if ((await page.locator("strong").textContent()) !== "Stream complete") {
    throw new Error("Suspense stream did not complete");
  }
  assertNoBrowserErrors();

  await page.goto("http://127.0.0.1:8080/about", { waitUntil: "networkidle" });
  if ((await page.locator("h1").textContent()) !== "About") {
    throw new Error("static page did not render or hydrate");
  }
  if ((await page.locator("main").evaluate((node) => getComputedStyle(node).paddingTop)) !== "16px") {
    throw new Error("Tailwind Vite plugin did not transform CSS");
  }

  await page.goto("http://127.0.0.1:8080/app", { waitUntil: "networkidle" });
  assertNoBrowserErrors();
  await assertImageAsset();
  const lazyFeature = page.locator('[data-lazy="vite-dynamic-import"]');
  await lazyFeature.waitFor();
  if ((await lazyFeature.textContent()) !== "Lazy loaded") {
    throw new Error("Vite dynamic import did not load");
  }
  const button = page.locator("button");
  await button.click();
  if ((await button.textContent()) !== "Count 1") {
    throw new Error("client page did not mount or update");
  }
  if ((await button.getAttribute("data-copy")) !== "counter") {
    throw new Error("JSON import did not load");
  }
  if ((await button.getAttribute("data-plugin")) !== "vite-plugin") {
    throw new Error("Vite virtual-module plugin did not run");
  }
  if ((await button.evaluate((node) => getComputedStyle(node).fontWeight)) !== "700") {
    throw new Error("CSS Module did not load");
  }
  const routeLink = page.locator("nav a");
  await routeLink.waitFor();
  if ((await routeLink.getAttribute("href")) !== "/post/first") {
    throw new Error("virtual routes module did not generate the link href");
  }
  if (Number(await routeLink.getAttribute("data-routes")) !== 5) {
    throw new Error("virtual routes module has an unexpected route count");
  }
  await routeLink.click();
  await page.waitForURL("**/post/first");
  if ((await page.locator("h1").textContent()) !== "First") {
    throw new Error("virtual routes link did not navigate to a working page");
  }
  assertNoBrowserErrors();
} finally {
  await browser.close();
}

console.log("browser integration passed");
