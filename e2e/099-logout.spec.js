// e2e/099-logout.spec.js - Final cleanup test
const { test, expect } = require("@playwright/test");
const { TestHelpers } = require("./test-helpers");
const { TEST_EMAIL, TEST_PASSWORD } = require("./test-constants");

test.describe("Logout", () => {
  let helpers;

  test.beforeEach(async ({ page }) => {
    helpers = new TestHelpers(page);
  });

  test.afterEach(async ({ page }) => {
    if (helpers) {
      await helpers.cleanup();
    }
  });

  test("1. Logout successfully", async ({ page }) => {
    helpers.log("=== Testing Logout ===");

    // Clear cookies and login
    await page.context().clearCookies();
    await helpers.login(TEST_EMAIL, TEST_PASSWORD);

    // Logout using the helper (POST request)
    await helpers.logout();

    // Verify we're on login page
    await expect(page).toHaveURL(/\/admin\/login/);
    helpers.log("✅ Logged out successfully and redirected to login page");

    // Verify we cannot access protected pages without login
    await page.goto("/admin/forms");
    await page.waitForLoadState("networkidle");
    await expect(page).toHaveURL(/\/admin\/login/);
    helpers.log("✅ Protected pages redirect to login");
  });
});
