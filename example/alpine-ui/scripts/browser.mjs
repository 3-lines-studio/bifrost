import assert from "node:assert/strict";
import { chromium } from "playwright-core";

const base = "http://127.0.0.1:18931";
const browser = await chromium.launch();
const page = await browser.newPage();
const errors = [];
page.on("pageerror", (error) => errors.push(error.message));

const check = (name, condition) => {
  assert.ok(condition, name);
  console.log(`  ok   ${name}`);
};

try {
  await page.goto(`${base}/`, { waitUntil: "load" });
  await page.waitForTimeout(300);
  check("alpine loaded", await page.evaluate(() => !!window.Alpine));
  check("ui css", (await page.locator("body").evaluate((el) => getComputedStyle(el).backgroundColor)) === "oklch(0.145 0 0)");

  const dialog = page.locator("dialog");
  const trigger = page.getByRole("button", { name: "Abrir dialog" });
  check("dialog starts closed", !(await dialog.evaluate((el) => el.open)));
  await trigger.click();
  await page.waitForTimeout(100);
  check("dialog opens", await dialog.evaluate((el) => el.open));
  check("dialog title", await page.getByRole("heading", { name: "¿Eliminar álbum?" }).isVisible());
  await page.keyboard.press("Escape");
  await page.waitForTimeout(100);
  check("dialog escape", !(await dialog.evaluate((el) => el.open)));
  check("dialog focus returns", (await page.evaluate(() => document.activeElement?.textContent)) === "Abrir dialog");

  const menu = page.getByRole("button", { name: "Acciones" });
  await menu.click();
  await page.waitForTimeout(100);
  check("menu opens", await page.getByRole("menu").isVisible());
  check("menu click focus", (await page.evaluate(() => document.activeElement?.textContent)) === "Editar");
  await page.keyboard.press("ArrowDown");
  await page.waitForTimeout(100);
  check("menu arrow focus", (await page.evaluate(() => document.activeElement?.textContent)) === "Compartir");
  await page.keyboard.press("Escape");
  await page.waitForTimeout(100);
  check("menu escape", !(await page.getByRole("menu").isVisible()));
  check("menu focus returns", (await page.evaluate(() => document.activeElement?.textContent)) === "Acciones");
  await menu.click();
  await page.waitForTimeout(100);
  await page.locator("h1").click();
  await page.waitForTimeout(100);
  check("menu outside closes", !(await page.getByRole("menu").isVisible()));
  check("outside keeps focus", (await page.evaluate(() => document.activeElement?.textContent)) !== "Acciones");

  await page.getByRole("tab", { name: "Facturación" }).click();
  await page.waitForTimeout(100);
  check("tabs click", (await page.getByRole("tab", { name: "Facturación" }).getAttribute("aria-selected")) === "true");
  check("tabs panel", await page.getByText("Contenido de facturación.").isVisible());
  await page.getByRole("tab", { name: "Facturación" }).press("ArrowLeft");
  await page.waitForTimeout(100);
  check("tabs arrow", (await page.getByRole("tab", { name: "Cuenta" }).getAttribute("aria-selected")) === "true");
  check("alert", await page.getByText("Contrato shadcn", { exact: true }).isVisible());
  check("table", (await page.locator(".ui-table__row").count()) === 3);

  await page.getByRole("button", { name: "Mostrar toast" }).click();
  await page.waitForTimeout(100);
  check("toast appears", (await page.locator(".ui-toast").count()) === 1);
  check("toast message", (await page.locator(".ui-toast__message").textContent()) === "Guardado");
  await page.getByRole("button", { name: "Dismiss notification" }).click();
  await page.waitForTimeout(100);
  check("toast dismiss", (await page.locator(".ui-toast").count()) === 0);

  check("no page errors", errors.length === 0);
  console.log("all browser checks passed");
} finally {
  await browser.close();
}
