import { chromium } from "playwright-core";

const url = process.env.WAILS_TEST_URL || "http://127.0.0.1:18081";
const executablePath = process.env.CHROMIUM_PATH || "/usr/bin/chromium";
const browser = await chromium.launch({ executablePath, headless: true });
try {
  const page = await browser.newPage();
  const errors = [];
  page.on("pageerror", (error) => {
    errors.push(error.message);
  });
  page.on("console", (message) => {
    if (message.type() === "error") {
      errors.push(message.text());
    }
  });

  await page.goto(url, { waitUntil: "domcontentloaded" });
  await page.getByPlaceholder("Don Berti").fill("Don Berti");
  await page.getByRole("button", { name: "Call Go" }).click();
  await page.getByText("Hello, Don Berti.").waitFor();
  await page.getByRole("button", { name: "Settings" }).click();
  await page.reload({ waitUntil: "domcontentloaded" });
  await page.getByText("The catch-all Bifrost route mounts this same application after a direct reload.").waitFor();

  if (errors.length > 0) {
    throw new Error(errors.join("\n"));
  }
} finally {
  await browser.close();
}
