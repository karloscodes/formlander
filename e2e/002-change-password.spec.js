// e2e/002-change-password.spec.js - Test password change functionality
const { test, expect } = require("@playwright/test");
const { TestHelpers } = require("./test-helpers");
const { TEST_EMAIL, TEST_PASSWORD } = require("./test-constants");

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

  test("1. Change password and change back", async ({ page }) => {
    helpers.log("=== Testing Password Change ===");

    const TEMP_PASSWORD = "temporary-password-123!";

    // Login with current password
    await helpers.login(TEST_EMAIL, TEST_PASSWORD);

    // Navigate to change password page
    await helpers.navigateTo("/admin/change-password");

    // Change to temporary password
    await page.fill('input[name="current_password"]', TEST_PASSWORD);
    await page.fill('input[name="new_password"]', TEMP_PASSWORD);
    await page.fill('input[name="confirm_password"]', TEMP_PASSWORD);

    await page.click('button[type="submit"]');
    await page.waitForLoadState("networkidle");
    await expect(page).toHaveURL(/\/admin(\/)?$/, { timeout: 10000 });

    helpers.log("✅ Password changed to temporary password");

    // Verify we're redirected
    let url = page.url();
    expect(url).not.toContain("/change-password");

    // Logout
    await helpers.logout();

    // Login with temporary password
    await helpers.login(TEST_EMAIL, TEMP_PASSWORD);
    helpers.log("✅ Temporary password works");

    // Change back to original password
    await helpers.navigateTo("/admin/change-password");
    await page.fill('input[name="current_password"]', TEMP_PASSWORD);
    await page.fill('input[name="new_password"]', TEST_PASSWORD);
    await page.fill('input[name="confirm_password"]', TEST_PASSWORD);

    await page.click('button[type="submit"]');
    await page.waitForLoadState("networkidle");
    await expect(page).toHaveURL(/\/admin(\/)?$/, { timeout: 10000 });

    helpers.log("✅ Password changed back to original");

    // Verify
    url = page.url();
    expect(url).not.toContain("/change-password");
  });

  test("2. Verify password is back to original", async ({ page }) => {
    helpers.log("=== Verifying Original Password Works ===");

    // Login with original password
    await helpers.login(TEST_EMAIL, TEST_PASSWORD);

    // Verify we're logged in
    await page.waitForLoadState("networkidle");
    const url = page.url();
    expect(url).toContain("/admin");
    expect(url).not.toContain("/login");

    helpers.log("✅ Original password verified");
  });
});
