// e2e/playwright.config.js
const path = require("node:path");
const fs = require("node:fs");

const projectRoot = path.resolve(__dirname, "..");
const browsersCacheDir =
  process.env.PLAYWRIGHT_BROWSERS_PATH ||
  path.join(projectRoot, "tmp", "ms-playwright");

// Stick to a repo-local browsers cache so we don't need access to ~/Library.
if (!fs.existsSync(browsersCacheDir)) {
  fs.mkdirSync(browsersCacheDir, { recursive: true });
}
process.env.PLAYWRIGHT_BROWSERS_PATH = browsersCacheDir;

const TEST_PORT = Number(process.env.PLAYWRIGHT_TEST_PORT || 41817);
const BASE_URL =
  process.env.PLAYWRIGHT_BASE_URL || `http://127.0.0.1:${TEST_PORT}`;

const { devices } = require("@playwright/test");

// Make sure we're using the test environment
process.env.FORMLANDER_ENV = "test";

module.exports = {
  testDir: "./",
  testMatch: [
    "**/*.spec.js"
  ], // All tests run in numeric order (001, 002, etc.)
  fullyParallel: false, // Sequential execution due to test dependencies
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0, // Retry once on CI only
  workers: 1, // Single worker - tests have dependencies (onboarding must run first)
  reporter: process.env.CI ? "github" : "line", // Faster line reporter locally
  timeout: 45000, // 45s timeout
  expect: {
    timeout: 8000, // 8s expect timeout
  },
  use: {
    baseURL: BASE_URL,
    trace: process.env.CI ? "on-first-retry" : "off", // No trace locally for speed
    video: "off", // Disabled for speed - use screenshots
    screenshot: "only-on-failure",
    actionTimeout: 10000, // 10s action timeout
    navigationTimeout: 20000, // 20s navigation timeout
    extraHTTPHeaders: {
      "X-Test-Source": "playwright-e2e",
    },
  },
  projects: [
    {
      name: "chromium",
      use: {
        ...devices["Desktop Chrome"],
        // Add viewport for consistency
        viewport: { width: 1280, height: 720 },
        // Disable animations for more reliable tests
        reducedMotion: "reduce",
      },
    },
  ],
  // Run your local dev server before starting the tests.
  webServer: {
    command:
      `cd .. && rm -f storage/formlander.test.db* && mkdir -p tmp/go-cache && GOCACHE=$(pwd)/tmp/go-cache FORMLANDER_ENV=test FORMLANDER_PORT=${TEST_PORT} LOG_LEVEL=info go run cmd/formlander/main.go`,
    url: `${BASE_URL}/_health`,
    reuseExistingServer: !process.env.CI,
    timeout: 90000, // 90s server startup
    ignoreHTTPSErrors: true,
    retries: 2,
  },
  // Global setup and teardown
  globalSetup: require.resolve("./setup-test-env.js"),
};
