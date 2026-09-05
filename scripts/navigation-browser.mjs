import assert from "node:assert/strict";
import { chromium } from "playwright-core";

const browser = await chromium.launch({
  executablePath: process.env.CHROMIUM_PATH || "/usr/bin/chromium",
  headless: true,
});
const origin = "http://127.0.0.1:18109";
try {
  const page = await browser.newPage();
  const errors = [];
  let documents = 0;
  page.on("pageerror", error => errors.push(error.message));
  page.on("request", request => {
    if (request.isNavigationRequest() && request.frame() === page.mainFrame()) {
      documents++;
    }
  });
  await page.goto(origin);
  await page.getByRole("button", { name: "Root 0", exact: true }).click();
  await page.getByRole("button", { name: "Root 1", exact: true }).waitFor();
  await page.evaluate(() => { window.navigationMarker = "alive"; });

  async function click(name, heading) {
    await page.getByRole("link", { name, exact: true }).click();
    await page.getByRole("heading", { name: heading, exact: true }).waitFor();
    await page.waitForFunction(() => !document.getElementById("app").hasAttribute("aria-busy"));
  }

  await click("One", "one");
  assert.equal(await page.locator("html").evaluate(node => node.classList.contains("dark")), true);
  await page.evaluate(() => document.documentElement.classList.add("client-class"));
  assert.equal(await page.title(), "one");
  assert.equal(await page.locator('meta[name="description"]').count(), 1);
  assert.equal(await page.locator('meta[name="description"]').getAttribute("content"), "one");
  assert.equal(await page.locator("#middleware").textContent(), "passed");
  assert.equal(await page.locator("h1").evaluate(node => getComputedStyle(node).color), "rgb(120, 30, 40)");
  assert.equal(await page.locator("html").getAttribute("lang"), "es");
  assert.equal(await page.locator("html").evaluate(node => node.classList.contains("post")), true);
  assert.equal(await page.locator("html").getAttribute("dir"), "ltr");
  assert.equal(await page.evaluate(() => document.activeElement.tagName), "H1");
  await page.getByRole("button", { name: "Posts 0", exact: true }).click();
  await page.getByRole("button", { name: "Posts 1", exact: true }).waitFor();
  await page.getByRole("button", { name: "Page 0", exact: true }).click();
  await page.getByRole("button", { name: "Page 1", exact: true }).waitFor();
  await page.locator("#draft").fill("unsaved draft");
  await page.evaluate(() => window.navigationAPI.navigate("?q=query"));
  assert.equal(new URL(page.url()).search, "?q=query");
  assert.equal(await page.locator("#query").textContent(), "query");
  await page.getByRole("button", { name: "Page 1", exact: true }).waitFor();
  assert.equal(await page.locator("#draft").inputValue(), "unsaved draft");
  const historyBeforeRefresh = await page.evaluate(() => ({ length: history.length, state: history.state }));
  const revision = Number(await page.locator("#revision").textContent());
  await page.locator("#draft").focus();
  await page.evaluate(() => scrollTo(0, 500));
  await page.waitForFunction(() => scrollY === 500);
  await page.evaluate(() => document.querySelector("form").requestSubmit());
  await page.waitForFunction(value => Number(document.getElementById("revision").textContent) === value + 1, revision);
  assert.equal(await page.evaluate(() => document.activeElement.id), "draft");
  assert.equal(await page.evaluate(() => scrollY), 500);
  await page.evaluate(() => window.navigationAPI.refresh());
  assert.deepEqual(await page.evaluate(() => ({ length: history.length, state: history.state })), historyBeforeRefresh);
  assert.equal(await page.evaluate(() => document.activeElement.id), "draft");
  assert.equal(await page.evaluate(() => scrollY), 500);
  assert.equal(await page.locator("#draft").inputValue(), "unsaved draft");
  await page.getByRole("button", { name: "Page 1", exact: true }).waitFor();
  await page.getByRole("button", { name: "Root 1", exact: true }).waitFor();
  await page.getByRole("button", { name: "Posts 1", exact: true }).waitFor();
  await page.evaluate(() => window.navigationAPI.navigate("/posts/one"));
  await page.goBack();
  await page.waitForFunction(() => document.getElementById("query").textContent === "query");
  await page.getByRole("button", { name: "Page 1", exact: true }).waitFor();
  assert.equal(await page.locator("#draft").inputValue(), "unsaved draft");
  await page.goForward();
  await page.waitForFunction(() => document.getElementById("query").textContent === "");
  await page.getByRole("button", { name: "Page 1", exact: true }).waitFor();
  await page.evaluate(() => scrollTo(0, 700));
  await page.waitForFunction(() => scrollY === 700);
  await page.evaluate(() => document.querySelector('a[href="/posts/two?q=yes"]').click());
  await page.getByRole("heading", { name: "two", exact: true }).waitFor();
  assert.equal(await page.locator("#query").textContent(), "yes");
  await page.getByRole("button", { name: "Page 0", exact: true }).waitFor();
  assert.equal(await page.locator("#draft").inputValue(), "");
  assert.equal(await page.locator("html").evaluate(node => node.classList.contains("client-class")), true);
  assert.equal(await page.locator("html").evaluate(node => node.classList.contains("dark")), true);
  assert.equal(await page.evaluate(() => scrollY), 0);
  await page.getByRole("button", { name: "Posts 1", exact: true }).waitFor();
  await page.goBack();
  await page.getByRole("heading", { name: "one", exact: true }).waitFor();
  await page.waitForFunction(() => scrollY === 700);
  await page.getByRole("button", { name: "Page 0", exact: true }).waitFor();
  assert.equal(await page.locator("#draft").inputValue(), "");
  await page.goForward();
  await page.getByRole("heading", { name: "two", exact: true }).waitFor();

  const beforeCancelledNavigation = await page.evaluate(() => history.length);
  const cancelledRequest = page.waitForRequest(request => request.url().endsWith("/posts/slow"));
  await page.evaluate(() => { window.pendingNavigation = window.navigationAPI.navigate("/posts/slow"); });
  await cancelledRequest;
  await page.evaluate(async () => {
    await window.navigationAPI.refresh();
    await window.pendingNavigation;
  });
  assert.equal(new URL(page.url()).pathname, "/posts/two");
  assert.equal(await page.evaluate(() => history.length), beforeCancelledNavigation);
  const invalidURLs = await page.evaluate(async () => {
    const results = [];
    for (const href of ["javascript:window.unsafeNavigation = true", "data:text/html,unsafe"]) {
      try {
        await window.navigationAPI.navigate(href);
        results.push(false);
      } catch (error) {
        results.push(error instanceof TypeError);
      }
    }
    return results;
  });
  assert.deepEqual(invalidURLs, [true, true]);
  assert.equal(await page.evaluate(() => window.unsafeNavigation), undefined);
  const delayedRefresh = page.waitForRequest(request => request.url().endsWith("/posts/two?q=yes"));
  await page.evaluate(() => {
    document.cookie = "navigation-delay=1; path=/";
    window.pendingRefresh = window.navigationAPI.refresh();
  });
  await delayedRefresh;
  await page.evaluate(async () => {
    document.cookie = "navigation-delay=; Max-Age=0; path=/";
    await window.navigationAPI.navigate("/posts/one");
    await window.pendingRefresh;
  });
  assert.equal(new URL(page.url()).pathname, "/posts/one");
  await page.waitForTimeout(2100);
  assert.equal(new URL(page.url()).pathname, "/posts/one");
  await page.getByRole("button", { name: "Page 0", exact: true }).click();
  await page.getByRole("button", { name: "Page 1", exact: true }).waitFor();
  await page.route("**/posts/one", async route => {
    await route.fulfill({ status: 302, headers: { Location: "/posts/two?q=refresh" } });
  });
  const beforeRefreshRedirect = await page.evaluate(() => history.length);
  await page.evaluate(() => window.navigationAPI.refresh());
  await page.unroute("**/posts/one");
  assert.equal(new URL(page.url()).search, "?q=refresh");
  assert.equal(await page.evaluate(() => history.length), beforeRefreshRedirect);
  assert.equal(await page.evaluate(() => document.activeElement.tagName), "H1");
  await page.getByRole("button", { name: "Page 0", exact: true }).waitFor();
  await page.getByRole("button", { name: "Posts 1", exact: true }).waitFor();


  const slow = page.waitForRequest(request => request.url().endsWith("/posts/slow"));
  await page.getByRole("link", { name: "Slow", exact: true }).click();
  await slow;
  assert.equal(await page.getByRole("heading", { name: "two", exact: true }).count(), 1);
  assert.equal(await page.locator("#app").getAttribute("aria-busy"), "true");
  await click("One", "one");
  await page.waitForTimeout(2100);
  assert.equal(new URL(page.url()).pathname, "/posts/one");
  await page.getByRole("button", { name: "Go to two", exact: true }).click();
  await page.waitForURL(origin + "/posts/two?from=action");
  await page.getByRole("heading", { name: "two", exact: true }).waitFor();
  await page.getByRole("button", { name: "Page 0", exact: true }).waitFor();
  await click("Redirect", "two");
  assert.equal(new URL(page.url()).search, "?q=redirect");
  await click("Missing", "Not found");
  await click("Broken", "Failed");
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await click("Unknown", "Not found");
  await click("Home", "Home");
  assert.equal(await page.title(), "Home");
  assert.equal(await page.locator("html").getAttribute("lang"), "en");
  assert.equal(await page.locator("html").getAttribute("dir"), null);
  await click("One", "one");
  await page.getByRole("button", { name: "Root 1", exact: true }).waitFor();
  await page.getByRole("button", { name: "Posts 0", exact: true }).waitFor();
  await page.getByRole("button", { name: "Page 0", exact: true }).click();
  await page.getByRole("button", { name: "Page 1", exact: true }).waitFor();
  await page.evaluate(() => scrollTo(0, 400));
  await page.waitForFunction(() => scrollY === 400);
  await page.evaluate(() => document.querySelector('a[href="/posts/one#anchor"]').click());
  await page.waitForFunction(() => Math.abs(document.getElementById("anchor").getBoundingClientRect().top) < 1);
  await page.goBack();
  await page.waitForFunction(() => scrollY === 400);
  await page.goForward();
  await page.waitForFunction(() => Math.abs(document.getElementById("anchor").getBoundingClientRect().top) < 1);
  const beforeSameHash = await page.evaluate(() => history.length);
  await page.evaluate(() => window.navigationAPI.navigate("#anchor"));
  assert.equal(await page.evaluate(() => history.length), beforeSameHash);
  await page.getByRole("button", { name: "Page 1", exact: true }).waitFor();
  assert.equal(await page.evaluate(() => window.navigationMarker), "alive");
  assert.equal(documents, 1);

  const modifiedClicks = await page.evaluate(() => {
    return [{ ctrlKey: true }, { metaKey: true }, { shiftKey: true }, { altKey: true }, { button: 1 }].map(options => {
      let intercepted;
      document.addEventListener("click", event => {
        intercepted = event.defaultPrevented;
        event.preventDefault();
      }, { once: true });
      document.querySelector('a[href="/posts/one"]').dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, ...options }));
      return intercepted;
    });
  });
  assert.deepEqual(modifiedClicks, [false, false, false, false, false]);

  const popupPromise = page.waitForEvent("popup");
  await page.getByRole("link", { name: "New tab", exact: true }).click();
  const popup = await popupPromise;
  await popup.waitForLoadState("domcontentloaded");
  assert.equal(new URL(popup.url()).pathname, "/posts/two");
  await popup.close();
  const downloadPromise = page.waitForEvent("download");
  await page.getByRole("link", { name: "Download", exact: true }).click();
  await downloadPromise;
  await page.getByRole("link", { name: "Reload", exact: true }).click();
  await page.waitForFunction(() => window.navigationMarker === undefined);
  assert.equal(documents, 2);
  await page.getByRole("button", { name: "Root 0", exact: true }).click();
  await page.getByRole("button", { name: "Root 1", exact: true }).waitFor();
  await page.getByRole("link", { name: "File", exact: true }).click();
  await page.waitForURL(origin + "/file.txt");
  assert.equal(documents, 3);
  assert.match(await page.locator("body").textContent(), /file body/);

  await page.goto(origin);
  await page.getByRole("button", { name: "Root 0", exact: true }).click();
  await page.getByRole("button", { name: "Root 1", exact: true }).waitFor();
  await page.route("**/posts/one", async route => {
    if (route.request().headers().accept !== "application/vnd.bifrost.navigation+json") {
      await route.continue();
      return;
    }
    const response = await route.fetch();
    const data = await response.json();
    await route.fulfill({ response, json: { ...data, build: "different-build" } });
  });
  const before = documents;
  await click("One", "one");
  assert.equal(documents, before + 1);
  await page.getByRole("button", { name: "Root 0", exact: true }).click();
  await page.getByRole("button", { name: "Root 1", exact: true }).waitFor();
  const beforeRefreshFallback = await page.evaluate(() => history.length);
  await page.evaluate(() => { window.navigationMarker = "refresh-fallback"; });
  await page.evaluate(() => { window.navigationAPI.refresh(); });
  await page.waitForFunction(() => window.navigationMarker === undefined);
  await page.getByRole("heading", { name: "one", exact: true }).waitFor();
  assert.equal(await page.evaluate(() => history.length), beforeRefreshFallback);
  assert.equal(documents, before + 2);

  assert.deepEqual(errors, []);

  const noJS = await browser.newContext({ javaScriptEnabled: false });
  const plain = await noJS.newPage();
  await plain.goto(origin);
  await plain.getByRole("link", { name: "One", exact: true }).click();
  await plain.getByRole("heading", { name: "one", exact: true }).waitFor();
  assert.equal(await plain.title(), "one");
  await noJS.close();
} finally {
  await browser.close();
}
