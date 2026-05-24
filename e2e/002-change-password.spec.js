// e2e/002-change-password.spec.js - Test password change via Settings
const { test, expect } = require("@playwright/test");
const { TestHelpers } = require("./test-helpers");
const { TEST_EMAIL, TEST_PASSWORD } = require("./test-constants");

const PW_FORM = 'form[action="/admin/settings/password"]';

test.describe("Change Password", () => {
  let helpers;

  test.beforeEach(async ({ page }) => {
    helpers = new TestHelpers(page);
    await page.context().clearCookies();
  });

  test.afterEach(async ({ page }) => {
    if (helpers) {
      await helpers.cleanup();
    }
  });

  async function changePassword(page, current, next) {
    await helpers.navigateTo("/admin/settings");
    await page.fill(`${PW_FORM} input[name="current_password"]`, current);
    await page.fill(`${PW_FORM} input[name="new_password"]`, next);
    await page.fill(`${PW_FORM} input[name="confirm_password"]`, next);
    await page.click(`${PW_FORM} button[type="submit"]`);
    await page.waitForLoadState("networkidle");
    await expect(page.locator("text=Password updated successfully")).toBeVisible({
      timeout: 10000,
    });
  }

  test("1. Change password and change back", async ({ page }) => {
    helpers.log("=== Testing Password Change (Settings) ===");

    const TEMP_PASSWORD = "temporary-password-123!";

    // Login with current password, change to a temporary one via Settings.
    await helpers.login(TEST_EMAIL, TEST_PASSWORD);
    await changePassword(page, TEST_PASSWORD, TEMP_PASSWORD);
    helpers.log("✅ Password changed to temporary password");

    // Re-login with the temporary password to prove it took effect.
    await helpers.logout();
    await helpers.login(TEST_EMAIL, TEMP_PASSWORD);
    helpers.log("✅ Temporary password works");

    // Change back to the original so later specs keep working.
    await changePassword(page, TEMP_PASSWORD, TEST_PASSWORD);
    helpers.log("✅ Password changed back to original");
  });

  test("2. Verify password is back to original", async ({ page }) => {
    helpers.log("=== Verifying Original Password Works ===");

    await helpers.login(TEST_EMAIL, TEST_PASSWORD);

    await page.waitForLoadState("networkidle");
    const url = page.url();
    expect(url).toContain("/admin");
    expect(url).not.toContain("/login");

    helpers.log("✅ Original password verified");
  });
});
