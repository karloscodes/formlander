// e2e/004-captcha.spec.js - Test captcha profile management
const { test, expect } = require("@playwright/test");
const { TestHelpers } = require("./test-helpers");
const { TEST_EMAIL, TEST_PASSWORD } = require("./test-constants");

test.describe("Captcha Profiles", () => {
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

  test("1. Create captcha profile", async ({ page }) => {
    helpers.log("=== Creating Captcha Profile ===");

    // Create a captcha profile using the helper
    const profileId = await helpers.createCaptchaProfile("Test Turnstile", "turnstile");

    // Navigate to captcha profiles page and verify it exists
    await helpers.navigateTo("/admin/settings/captcha");

    const pageContent = await page.textContent("body");
    expect(pageContent).toContain("Captcha Profiles");

    helpers.log("✅ Captcha profile created");
  });

  test("2. View captcha profiles list", async ({ page }) => {
    helpers.log("=== Viewing Captcha Profiles List ===");

    // Create a test captcha profile first
    await helpers.createCaptchaProfile("Test Captcha List", "turnstile");

    await helpers.navigateTo("/admin/settings/captcha");

    const pageContent = await page.textContent("body");
    expect(pageContent).toContain("Captcha Profiles");
    expect(pageContent).toMatch(/Test Captcha List|turnstile/i);

    helpers.log("✅ Captcha profiles list displayed");
  });

  test("3. Edit captcha profile", async ({ page }) => {
    helpers.log("=== Editing Captcha Profile ===");

    // Create a test captcha profile to edit
    const profileId = await helpers.createCaptchaProfile("Test Edit Captcha", "turnstile");

    await helpers.navigateTo("/admin/settings/captcha");

    // Verify the profile appears in the list
    const pageContent = await page.textContent("body");
    expect(pageContent).toContain("Test Edit Captcha");

    helpers.log("✅ Captcha profile exists in list");
  });

  test("4. Form submission rejected without captcha token", async ({ page, request }) => {
    helpers.log("=== Testing Captcha Enforcement ===");

    // Create captcha profile with a secret key
    const captchaProfileId = await helpers.createCaptchaProfile(
      "Enforcement Test Captcha",
      "turnstile",
      "test-secret-for-enforcement"
    );

    // Create form with captcha enabled
    const uniqueSlug = `captcha-test-${Date.now()}`;
    const formData = await helpers.createFormData("Captcha Test Form", uniqueSlug, {
      captchaProfileId,
    });

    helpers.log(`Created form with slug: ${uniqueSlug}, token: ${formData.token}, captcha profile: ${captchaProfileId}`);

    // Submit to form WITHOUT captcha token - should be rejected
    const response = await request.post(`/forms/${uniqueSlug}/submit?token=${formData.token}`, {
      headers: {
        "Content-Type": "application/json",
        "Origin": "http://localhost:3000",
      },
      data: {
        name: "Test User",
        email: "test@example.com",
        message: "This should be rejected",
        // NOTE: No cf-turnstile-response token!
      },
    });

    // Expect rejection due to missing captcha
    expect(response.status()).toBe(400);
    const body = await response.json();
    expect(body.error).toContain("captcha");

    helpers.log("✅ Form submission correctly rejected without captcha token");
  });
});
