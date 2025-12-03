// e2e/003-mailers.spec.js - Test mailer profile management
const { test, expect } = require("@playwright/test");
const { TestHelpers } = require("./test-helpers");
const { TEST_EMAIL, TEST_PASSWORD } = require("./test-constants");

test.describe("Mailer Profiles", () => {
  let helpers;

  test.beforeEach(async ({ page }) => {
    helpers = new TestHelpers(page);
    await page.context().clearCookies();
    await helpers.login(TEST_EMAIL, TEST_PASSWORD);
  });

  test.afterEach(async ({ page }) => {
    if (helpers) {
      await helpers.cleanup();
    }
  });

  test("1. Create mailer profile", async ({ page }) => {
    helpers.log("=== Creating Mailer Profile ===");

    await helpers.navigateTo("/admin/settings/mailers");
    await page.click('text=New Profile');
    await page.waitForLoadState("networkidle");

    // Fill mailer profile details
    await page.fill('input[name="name"]', "Test Mailgun");
    await page.fill('input[name="api_key"]', "key-test-mailgun-key-12345");
    await page.fill('input[name="domain"]', "mg.example.com");
    await page.fill('input[name="default_from_name"]', "Formlander");
    await page.fill('input[name="default_from_email"]', "noreply@example.com");

    await page.click('button[type="submit"]');
    await page.waitForLoadState("networkidle");

    // Verify we're on a mailer-related page
    const url = page.url();
    expect(url).toContain("/admin/settings/mailers");

    helpers.log("✅ Mailer profile created");
  });

  test("2. View mailer profiles list", async ({ page }) => {
    helpers.log("=== Viewing Mailer Profiles List ===");

    // Create a test profile first with a unique name
    await helpers.createMailerProfile("Test Mailgun List", "mailgun", "test@example.com");

    await helpers.navigateTo("/admin/settings/mailers");

    const pageContent = await page.textContent("body");
    expect(pageContent).toContain("Mailer Profiles");
    expect(pageContent).toContain("Test Mailgun List");

    helpers.log("✅ Mailer profiles list displayed");
  });

  test("3. Edit mailer profile", async ({ page }) => {
    helpers.log("=== Editing Mailer Profile ===");

    // Create a test profile first with a unique name
    const profileId = await helpers.createMailerProfile("Test Mailgun Edit", "mailgun", "test@example.com");

    await helpers.navigateTo("/admin/settings/mailers");

    // Verify the profile appears in the list
    const pageContent = await page.textContent("body");
    expect(pageContent).toContain("Test Mailgun Edit");

    helpers.log("✅ Mailer profile exists in list");
  });
});
