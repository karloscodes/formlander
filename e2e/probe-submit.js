// Submits a form on a live page in a real browser and prints what the
// browser sent and what Formlander answered. Use it when a form on an
// allowed site is rejected.
//
//   node e2e/probe-submit.js <page-url> [form-selector] [field=value ...]
//   node e2e/probe-submit.js https://formlander.com/cloud/ '#waitlist' email=probe@example.com
//
// The origin check runs before the captcha check, so a captcha error
// still proves that the origin was accepted.
const { chromium } = require("@playwright/test");

async function main() {
  const [pageURL, formSelector = "form", ...fields] = process.argv.slice(2);
  if (!pageURL) {
    console.error("usage: node e2e/probe-submit.js <page-url> [form-selector] [field=value ...]");
    process.exit(1);
  }

  const browser = await chromium.launch();
  const page = await browser.newPage();

  page.on("request", (req) => {
    if (req.method() !== "POST" || !req.url().includes("/forms/")) return;
    const headers = req.headers();
    console.log(`POST    ${req.url().split("?")[0]}`);
    console.log(`origin  ${headers.origin || "(none)"}`);
    console.log(`referer ${headers.referer || "(none)"}`);
  });
  page.on("response", (res) => {
    if (res.request().method() !== "POST" || !res.url().includes("/forms/")) return;
    console.log(`status  ${res.status()} ${res.headers().location || ""}`);
  });

  await page.goto(pageURL, { waitUntil: "load" });
  console.log(`action  ${await page.getAttribute(formSelector, "action")}`);

  for (const field of fields) {
    const [name, ...rest] = field.split("=");
    await page.fill(`${formSelector} [name="${name}"]`, rest.join("="));
  }

  await Promise.all([
    page.waitForNavigation({ timeout: 20000 }).catch(() => {}),
    page.click(`${formSelector} [type=submit]`),
  ]);

  const title = await page.$eval("h1", (el) => el.textContent).catch(() => "");
  const details = await page.$eval(".details", (el) => el.textContent).catch(() => "");
  console.log(`landed  ${page.url()}`);
  console.log(`result  ${[title, details].filter(Boolean).join(" | ") || "(no Formlander page)"}`);

  await browser.close();
}

main().catch((err) => {
  console.error(err.message);
  process.exit(1);
});
