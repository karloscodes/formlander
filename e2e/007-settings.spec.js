// e2e/007-settings.spec.js - Test settings management
const { test, expect } = require("@playwright/test");
const { TestHelpers } = require("./test-helpers");
const { TEST_EMAIL, TEST_PASSWORD } = require("./test-constants");

test.describe("Settings Management", () => {
  let helpers;

  test.beforeEach(async ({ page }) => {
    helpers = new TestHelpers(page);

    // Login
    await page.context().clearCookies();
    await helpers.login(TEST_EMAIL, TEST_PASSWORD);
  });

  test.afterEach(async ({ page }) => {
    if (helpers) {
      await helpers.cleanup();
    }
  });

  test("1. View settings page", async ({ page }) => {
    helpers.log("=== Viewing Settings Page ===");

    await helpers.navigateTo("/admin/settings");

    const pageContent = await page.textContent("body");
    expect(pageContent).toContain("Settings");
    expect(pageContent).toContain("Account");

    helpers.log("✅ Settings page loaded");
  });

  test("2. Manage mailer profiles", async ({ page }) => {
    helpers.log("=== Managing Mailer Profiles ===");

    // Navigate to mailers page
    await helpers.navigateTo("/admin/settings/mailers");

    // Should see the mailer profiles page (might be empty initially)
    const pageContent = await page.textContent("body");
    expect(pageContent).toContain("Mailer Profiles");

    helpers.log("✅ Mailer profiles page loaded");

    // Create a new mailer profile
    await page.click('text=New Profile');
    await page.waitForLoadState("networkidle");

    const mailerName = `test-mailgun-${Date.now()}`;

    // Fill out the new mailer form manually (select fields need special handling)
    await page.fill('input[name="name"]', mailerName);
    await page.selectOption('select[name="provider"]', "mailgun");
    await page.fill('input[name="api_key"]', "key-test-mailgun-key");
    await page.fill('input[name="domain"]', "mg.example.com");
    await page.fill('input[name="default_from_name"]', "Test Sender");
    await page.fill('input[name="default_from_email"]', `test+${Date.now()}@example.com`);
    await page.fill('textarea[name="defaults_json"]', '{"tags":["formlander-e2e"]}');

    await page.click('form[action="/admin/settings/mailers"] button[type="submit"]');
    await page.waitForURL("**/admin/settings/mailers");
    await page.waitForLoadState("networkidle");
    await expect(page.locator("body")).toContainText(mailerName);

    helpers.log("✅ Mailer profile created");
  });

  test("3. Manage captcha profiles", async ({ page }) => {
    helpers.log("=== Managing Captcha Profiles ===");

    // Navigate to captcha page
    await helpers.navigateTo("/admin/settings/captcha");

    // Should see the captcha profiles page (might be empty initially)
    const pageContent = await page.textContent("body");
    expect(pageContent).toContain("Captcha Profiles");

    helpers.log("✅ Captcha profiles page loaded");

    // Create a new captcha profile
    await page.click('text=New Captcha Profile');
    await page.waitForLoadState("networkidle");

    const captchaName = `test-turnstile-${Date.now()}`;
    const siteKey = `0xTEST-${Date.now()}`;

    // Fill out the new captcha form manually (select fields need special handling)
    await page.fill('input[name="name"]', captchaName);
    await page.selectOption('select[name="provider"]', "turnstile");
    await page.fill('input[name="secret_key"]', "1x0000000000000000000000000000000AA");
    await page.fill('textarea[name="site_keys_json"]', JSON.stringify([{ host_pattern: "*", site_key: siteKey }], null, 2));
    await page.fill('textarea[name="policy_json"]', JSON.stringify({ required: true, action: "submit" }, null, 2));

    await page.click('form[action="/admin/settings/captcha"] button[type="submit"]');
    await page.waitForURL("**/admin/settings/captcha");
    await page.waitForLoadState("networkidle");
    await expect(page.locator("body")).toContainText(captchaName);

    helpers.log("✅ Captcha profile created");
  });

  test("4. Change password again", async ({ page }) => {
    helpers.log("=== Changing Password Again ===");

    await helpers.navigateTo("/admin/change-password");

    await helpers.fillForm({
      current_password: TEST_PASSWORD,
      new_password: "newpassword456",
      confirm_password: "newpassword456",
    });

    // Wait for redirect to admin dashboard (successful password change redirects to /admin)
    await page.waitForURL("**/admin");
    await page.waitForLoadState("networkidle");
    helpers.log("✅ Password changed");

    // Logout and login with new password
    await helpers.logout();
    await helpers.login(TEST_EMAIL, "newpassword456");

    helpers.log("✅ Login with new password successful");

    // Change back to original password for other tests
    await helpers.navigateTo("/admin/change-password");
    await helpers.fillForm({
      current_password: "newpassword456",
      new_password: TEST_PASSWORD,
      confirm_password: TEST_PASSWORD,
    });

    // Wait for redirect to admin dashboard (successful password change)
    await page.waitForURL("**/admin");
    await page.waitForLoadState("networkidle");

    helpers.log("✅ Password changed back to original");
  });

  test("5. Change password via settings page", async ({ page }) => {
    helpers.log("=== Changing Password From Settings Page ===");

    const tempPassword = `TempPwd!${Date.now()}`;

    await helpers.navigateTo("/admin/settings");
    await helpers.fillForm(
      {
        current_password: TEST_PASSWORD,
        new_password: tempPassword,
        confirm_password: tempPassword,
      },
      { submitButton: 'form[action="/admin/settings/password"] button[type="submit"]' }
    );
    await expect(page.locator("body")).toContainText("Password updated successfully");

    await helpers.logout();
    await helpers.login(TEST_EMAIL, tempPassword);

    await helpers.navigateTo("/admin/settings");
    await helpers.fillForm(
      {
        current_password: tempPassword,
        new_password: TEST_PASSWORD,
        confirm_password: TEST_PASSWORD,
      },
      { submitButton: 'form[action="/admin/settings/password"] button[type="submit"]' }
    );
    await expect(page.locator("body")).toContainText("Password updated successfully");

    helpers.log("✅ Settings password change verified");
  });
});
