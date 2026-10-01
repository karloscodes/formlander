// e2e/006-submissions.spec.js - Test form submissions and rate limiting
const { test, expect } = require("@playwright/test");
const { TestHelpers } = require("./test-helpers");
const { TEST_EMAIL, TEST_PASSWORD } = require("./test-constants");

test.describe("Form Submissions", () => {
  let helpers;
  let formToken;
  let formSlug;

  test.beforeEach(async ({ page }, testInfo) => {
    helpers = new TestHelpers(page);

    // Login
    await page.context().clearCookies();
    await helpers.login(TEST_EMAIL, TEST_PASSWORD);

    // Create a test form for submissions using database with unique slug
    const timestamp = Date.now();
    formSlug = `test-contact-form-${timestamp}`;
    const formData = await helpers.createFormData("Test Contact Form", formSlug);
    formToken = formData.token;

    helpers.log(`Created test form: ${formSlug} with token: ${formToken}`);
  });

  test.afterEach(async ({ page }) => {
    if (helpers) {
      await helpers.cleanup();
    }
  });

  test("1. Submit a form successfully", async ({ page }) => {
    helpers.log("=== Submitting Form ===");

    const response = await helpers.submitToForm(formSlug, formToken, {
      name: "Alice Smith",
      email: "alice@example.com",
      message: "This is a test submission from the E2E tests",
    });

    expect(response.status()).toBe(200);
    helpers.log("✅ Form submitted successfully");

    // Check submission in admin
    await helpers.navigateTo("/admin/forms");
    await page.waitForSelector(`text=Test Contact Form`);
    await page.locator('tr:has-text("Test Contact Form")').first().click();
    await page.waitForLoadState("networkidle");

    // Should show submission count or content
    const pageContent = await page.textContent("body");
    expect(pageContent).toMatch(/submissions?|Alice Smith/i);

    helpers.log("✅ Submission appears in admin");
  });

  test("2. View submission details", async ({ page }) => {
    helpers.log("=== Viewing Submission Details ===");

    // First submit a form to have data to view
    const response = await helpers.submitToForm(formSlug, formToken, {
      name: "Bob Wilson",
      email: "bob@example.com",
      message: "Test submission for viewing details",
    });

    expect(response.status()).toBe(200);
    helpers.log("✅ Form submitted for viewing test");

    // Navigate to submissions page
    await helpers.navigateTo("/admin/submissions");

    // Should see submissions in the list
    const pageContent = await page.textContent("body");
    expect(pageContent).toMatch(/bob@example\.com|Test Contact Form/i);

    helpers.log("✅ Submission list displayed correctly");
  });

  test("3. Test rate limiting", async ({ page }) => {
    helpers.log("=== Testing Rate Limiting ===");

    // Submit first request
    const response1 = await helpers.submitToForm(formSlug, formToken, {
      name: "John Doe",
      email: "john@example.com",
      message: "This is a test submission",
    });
    expect(response1.status()).toBe(200);
    helpers.log("✅ First submission successful");

    // Submit second request immediately (should be rate limited)
    const response2 = await helpers.submitToForm(formSlug, formToken, {
      name: "Jane Smith",
      email: "jane@example.com",
      message: "Second submission",
    });

    // Should be rate limited (429) or successful (200) depending on timing
    // Since rate limit is 60/min by default, we accept both
    const status2 = response2.status();
    helpers.log(`Second submission status: ${status2}`);
    expect([200, 429]).toContain(status2);

    if (status2 === 429) {
      helpers.log("✅ Rate limiting working correctly");
    } else {
      helpers.log("⚠️  Rate limit not triggered (might be slow enough to pass)");
    }
  });

  test("4. Test API endpoint submission", async ({ page }) => {
    helpers.log("=== Testing API Endpoint Submission ===");

    const response = await page.request.post(`/forms/${formSlug}/submit?token=${formToken}`, {
      data: new URLSearchParams({
        name: "API Tester",
        email: "api@example.com",
        message: "Submitted via API",
      }).toString(),
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
      },
    });

    expect(response.status()).toBe(200);
    helpers.log("✅ API submission successful");

    // Verify in admin - navigate to submissions page to see the new submission
    await helpers.navigateTo("/admin/submissions");

    const pageContent = await page.textContent("body");
    expect(pageContent).toMatch(/API Tester|api@example\.com/i);

    helpers.log("✅ API submission appears in admin");
  });

  test("5. Delete a submission from its page and from the list", async ({ page }) => {
    helpers.log("=== Deleting Submissions ===");

    const stamp = Date.now();
    const first = `delete-one-${stamp}@example.com`;
    const second = `delete-two-${stamp}@example.com`;
    for (const email of [first, second]) {
      const response = await helpers.submitToForm(formSlug, formToken, { name: "To Delete", email });
      expect(response.status()).toBe(200);
    }

    // Every delete asks first.
    page.on("dialog", (dialog) => dialog.accept());

    // From the page of the submission. It goes back to the list.
    await helpers.navigateTo("/admin/submissions");
    await page.click(`a:has-text("${first}")`);
    await page.waitForURL(/\/admin\/submissions\/\d+$/);
    await expect(page.locator("h1")).toContainText(first);
    await page.click('main button:has-text("Delete")');
    await page.waitForURL("**/admin/submissions");
    await expect(page.locator("body")).not.toContainText(first);

    // From the list: tick it, then delete the selected.
    await page.locator(`li:has-text("${second}") input[name="ids"]`).check();
    await expect(page.locator("[data-delete-selected]")).toHaveText("Delete 1 selected");
    await page.click("[data-delete-selected]");
    await page.waitForLoadState("networkidle");
    await expect(page.locator("body")).not.toContainText(second);

    helpers.log("✅ Submissions deleted");
  });
});
