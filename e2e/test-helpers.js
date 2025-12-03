// e2e/test-helpers.js
const { expect } = require("@playwright/test");
const Database = require("sqlite3").verbose();
const bcrypt = require("bcrypt");
const path = require("path");

/**
 * Test helper utilities for E2E tests
 */

class TestHelpers {
  constructor(page) {
    this.page = page;
    this.db = null;
  }

  /**
   * Connect to test database
   */
  async connectDB() {
    if (this.db) return this.db;

    // Determine database path based on environment
    const dataDir = process.env.FORMLANDER_DATA_DIR || path.join(process.cwd(), "../storage");
    const dbFilename = process.env.FORMLANDER_DATABASE_FILENAME || "formlander.test.db";
    const dbPath = path.join(dataDir, dbFilename);

    this.log(`Connecting to database: ${dbPath}`);

    return new Promise((resolve, reject) => {
      this.db = new Database.Database(dbPath, (err) => {
        if (err) {
          this.log(`Database connection failed: ${err.message}`, "error");
          reject(err);
        } else {
          this.log("Database connected successfully");
          resolve(this.db);
        }
      });
    });
  }

  /**
   * Close database connection
   */
  async closeDB() {
    if (this.db) {
      return new Promise((resolve) => {
        this.db.close((err) => {
          if (err) {
            this.log(`Database close error: ${err.message}`, "warn");
          } else {
            this.log("Database connection closed");
          }
          this.db = null;
          resolve();
        });
      });
    }
  }

  /**
   * Execute SQL query
   */
  async execSQL(sql, params = []) {
    const db = await this.connectDB();
    return new Promise((resolve, reject) => {
      db.run(sql, params, function (err) {
        if (err) {
          reject(err);
        } else {
          resolve({ lastID: this.lastID, changes: this.changes });
        }
      });
    });
  }

  /**
   * Get single row from SQL query
   */
  async getSQL(sql, params = []) {
    const db = await this.connectDB();
    return new Promise((resolve, reject) => {
      db.get(sql, params, (err, row) => {
        if (err) {
          reject(err);
        } else {
          resolve(row);
        }
      });
    });
  }

  /**
   * Get all rows from SQL query
   */
  async allSQL(sql, params = []) {
    const db = await this.connectDB();
    return new Promise((resolve, reject) => {
      db.all(sql, params, (err, rows) => {
        if (err) {
          reject(err);
        } else {
          resolve(rows);
        }
      });
    });
  }

  /**
   * Create admin user if not exists
   */
  async createAdminUser(email = "admin@formlander.local", password = "formlander") {
    try {
      // Check if user exists
      const existingUser = await this.getSQL("SELECT id FROM users WHERE email = ?", [email]);
      if (existingUser) {
        this.log(`Admin user already exists: ${email}`);
        return existingUser.id;
      }

      // Create password hash
      const passwordHash = await bcrypt.hash(password, 12);
      const now = new Date().toISOString();

      // Create user
      const result = await this.execSQL(
        "INSERT INTO users (email, password_hash, last_login_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
        [email, passwordHash, now, now, now]
      );

      this.log(`✅ Created admin user: ${email}`);
      return result.lastID;
    } catch (error) {
      this.log(`Failed to create admin user: ${error.message}`, "error");
      throw error;
    }
  }

  /**
   * Create mailer profile
   */
  async createMailerProfile(name = "Test Mailer", provider = "mailgun", fromEmail = "test@example.com") {
    try {
      const now = new Date().toISOString();

      const result = await this.execSQL(
        `INSERT INTO mailer_profiles (name, provider, default_from_name, default_from_email, created_at, updated_at) 
         VALUES (?, ?, ?, ?, ?, ?)`,
        [name, provider, "Formlander", fromEmail, now, now]
      );

      this.log(`✅ Created mailer profile: ${name}`);
      return result.lastID;
    } catch (error) {
      this.log(`Failed to create mailer profile: ${error.message}`, "error");
      throw error;
    }
  }

  /**
   * Create captcha profile
   */
  async createCaptchaProfile(name = "Test Captcha", provider = "turnstile") {
    try {
      const now = new Date().toISOString();
      const siteKeys = JSON.stringify([{ "host_pattern": "*", "site_key": "" }]);
      const policy = JSON.stringify({ "required": false, "action": "submit", "widget": "managed" });

      const result = await this.execSQL(
        `INSERT INTO captcha_profiles (name, provider, site_keys_json, policy_json, created_at, updated_at) 
         VALUES (?, ?, ?, ?, ?, ?)`,
        [name, provider, siteKeys, policy, now, now]
      );

      this.log(`✅ Created captcha profile: ${name}`);
      return result.lastID;
    } catch (error) {
      this.log(`Failed to create captcha profile: ${error.message}`, "error");
      throw error;
    }
  }

  /**
   * Create form
   */
  async createFormData(name, slug, options = {}) {
    try {
      const {
        mailerProfileId = null,
        captchaProfileId = null,
        allowedOrigins = "*",
        emailEnabled = false,
        webhookEnabled = false,
        emailRecipient = null,
        webhookUrl = null
      } = options;

      const now = new Date().toISOString();

      // Generate secure token
      const token = require('crypto').randomBytes(32).toString('hex');

      // Generate public_id (similar to Go's generatePublicID)
      const uuid = require('crypto').randomUUID();
      const publicId = uuid.replace(/-/g, '').substring(0, 20);

      // Create form
      const formResult = await this.execSQL(
        `INSERT INTO forms (public_id, name, slug, token, allowed_origins, created_at, updated_at) 
         VALUES (?, ?, ?, ?, ?, ?, ?)`,
        [publicId, name, slug, token, allowedOrigins, now, now]
      );

      const formId = formResult.lastID;
      this.log(`✅ Created form: ${name} (ID: ${formId})`);

      // Create email delivery if needed
      if (emailEnabled && emailRecipient) {
        const overrides = JSON.stringify({ to: emailRecipient });
        await this.execSQL(
          `INSERT INTO email_deliveries (form_id, mailer_profile_id, enabled, overrides_json, created_at, updated_at) 
           VALUES (?, ?, ?, ?, ?, ?)`,
          [formId, mailerProfileId, 1, overrides, now, now]
        );
        this.log(`✅ Created email delivery for form: ${name}`);
      }

      // Create webhook delivery if needed
      if (webhookEnabled && webhookUrl) {
        await this.execSQL(
          `INSERT INTO webhook_deliveries (form_id, enabled, url, created_at, updated_at) 
           VALUES (?, ?, ?, ?, ?)`,
          [formId, 1, webhookUrl, now, now]
        );
        this.log(`✅ Created webhook delivery for form: ${name}`);
      }

      return { formId, token };
    } catch (error) {
      this.log(`Failed to create form: ${error.message}`, "error");
      throw error;
    }
  }

  /**
   * Enhanced logging with timestamps
   */
  log(message, level = "info") {
    const timestamp = new Date().toISOString();
    const prefix = level === "error" ? "❌" : level === "warn" ? "⚠️" : "ℹ️";
    console.log(`[${timestamp}] ${prefix} ${message}`);
  }

  /**
   * Wait for element with better error handling
   */
  async waitForElement(selector, options = {}) {
    const { timeout = 10000, state = "visible", silent = false } = options;
    if (!silent) {
      this.log(`Waiting for element: ${selector}`);
    }

    try {
      await this.page.waitForSelector(selector, { timeout, state });
      if (!silent) {
        this.log(`Element found: ${selector}`);
      }
      return true;
    } catch (error) {
      if (!silent) {
        this.log(`Element not found: ${selector} - ${error.message}`, "error");
      }
      throw error;
    }
  }

  /**
   * Enhanced navigation with validation
   */
  async navigateTo(path, options = {}) {
    const { waitForSelector = null, timeout = 30000 } = options;

    // Use relative paths so Playwright uses the configured baseURL
    const relativePath = path.startsWith('/') ? path : `/${path}`;
    this.log(`Navigating to: ${relativePath}`);

    try {
      const response = await this.page.goto(relativePath, { timeout, waitUntil: "domcontentloaded" });

      // Check for server errors
      if (response && !response.ok()) {
        this.log(`Server returned ${response.status()} for ${relativePath}`, "error");
        throw new Error(`Server error: ${response.status()}`);
      }

      await this.page.waitForLoadState("networkidle", { timeout: 10000 });
      await this.page.waitForTimeout(500); // Reduced wait time

      if (waitForSelector) {
        await this.waitForElement(waitForSelector);
      }

      this.log(`Navigation successful: ${this.page.url()}`);
    } catch (error) {
      this.log(`Navigation failed: ${relativePath} - ${error.message}`, "error");
      throw error;
    }
  }

  /**
   * Enhanced form filling with validation
   */
  async fillForm(formFields, options = {}) {
    const { submitButton = 'button[type="submit"]', waitAfterSubmit = true } = options;
    this.log(`Filling form with ${Object.keys(formFields).length} fields`);

    try {
      for (const [fieldName, value] of Object.entries(formFields)) {
        const selector = `input[name="${fieldName}"], input[id="${fieldName}"], textarea[name="${fieldName}"], textarea[id="${fieldName}"], select[name="${fieldName}"], select[id="${fieldName}"]`;
        await this.waitForElement(selector);
        await this.page.fill(selector, value);
        this.log(`Filled field: ${fieldName}`);
      }

      if (submitButton !== null && submitButton !== undefined) {
        await this.page.click(submitButton);
        this.log("Form submitted");

        if (waitAfterSubmit) {
          await this.page.waitForLoadState("networkidle");
        }
      } else {
        this.log("Form filled but not submitted (submitButton is null)");
      }
    } catch (error) {
      this.log(`Form filling failed: ${error.message}`, "error");
      throw error;
    }
  }

  /**
   * Enhanced login with comprehensive error handling
   */
  async login(email, password, options = {}) {
    const {
      expectSuccess = true,
      redirectPath = null,
      timeout = 30000,
      changePassword = null,
    } = options;

    this.log(`Attempting login: ${email}`);

    try {
      // Navigate to login page
      await this.navigateTo("/admin/login", {
        waitForSelector: 'input[name="email"]',
        timeout
      });

      // Clear any existing values and fill login form
      this.log("Clearing and filling form fields");
      await this.page.fill('input[name="email"]', '');
      await this.page.fill('input[name="password"]', '');
      await this.page.fill('input[name="email"]', email);
      await this.page.fill('input[name="password"]', password);

      // Check that form fields are correctly filled
      const emailValue = await this.page.inputValue('input[name="email"]');
      const passwordValue = await this.page.inputValue('input[name="password"]');
      this.log(`Form filled - Email: ${emailValue}, Password length: ${passwordValue.length}`);

      // Submit form - try both methods to be safe
      this.log("Submitting form");
      const submitPromise = this.page.waitForResponse(response =>
        response.url().includes('/admin/login') && response.request().method() === 'POST'
      );

      await this.page.click('button[type="submit"]');

      // Wait for the POST request to complete
      try {
        const response = await submitPromise;
        this.log(`Login POST response: ${response.status()} ${response.statusText()}`);
      } catch (e) {
        this.log(`No POST response captured: ${e.message}`);
      }

      // Wait for any navigation/redirect to complete
      await this.page.waitForLoadState("networkidle", { timeout: 15000 });
      await this.page.waitForTimeout(1000);

      const currentUrl = this.page.url();
      this.log(`Post-login URL: ${currentUrl}`);

      // Handle password change if required
      if (changePassword && currentUrl.includes("/admin/change-password")) {
        this.log("Password change required - updating password");
        await this.fillForm({
          current_password: password,
          new_password: changePassword,
          confirm_password: changePassword,
        });
        await this.page.waitForLoadState("networkidle");
        this.log("Password changed successfully");
      }

      if (expectSuccess) {
        // Should not be on login page
        expect(currentUrl).not.toContain("/admin/login");
        this.log(`✅ Login successful`);
      }

      if (redirectPath) {
        expect(currentUrl).toContain(redirectPath);
        this.log(`✅ Redirected to expected path: ${redirectPath}`);
      }

    } catch (error) {
      this.log(`Login failed: ${error.message}`, "error");
      // Take screenshot on login failure
      try {
        await this.page.screenshot({ path: `test-results/login-failure-${Date.now()}.png`, fullPage: true });
      } catch (screenshotError) {
        this.log(`Screenshot failed: ${screenshotError.message}`, "warn");
      }
      throw error;
    }
  }

  /**
   * Logout
   */
  async logout() {
    this.log("Logging out");
    try {
      // Click the logout button/link or POST to logout endpoint
      // First, try to find a logout button on the page
      const logoutButton = await this.page.locator('a[href="/admin/logout"], button:has-text("logout"), button:has-text("sign out")').first();
      const logoutExists = await logoutButton.count() > 0;

      if (logoutExists) {
        await logoutButton.click();
      } else {
        // If no button, manually POST to logout
        await this.page.evaluate(async () => {
          const form = document.createElement('form');
          form.method = 'POST';
          form.action = '/admin/logout';
          document.body.appendChild(form);
          form.submit();
        });
      }

      // Wait for redirect to login page
      await this.page.waitForURL(/\/admin\/login/, { timeout: 10000 });
      await this.page.waitForLoadState("networkidle");
      this.log("✅ Logout successful");
    } catch (error) {
      this.log(`Logout failed: ${error.message}`, "error");
      throw error;
    }
  }

  /**
   * Check for flash messages
   */
  async checkForMessages(type = "success") {
    const selector = `.flash-${type}, .alert-${type}`;
    try {
      const element = await this.page.waitForSelector(selector, { timeout: 5000 });
      const text = await element.textContent();
      this.log(`Found ${type} message: ${text}`);
      return text;
    } catch (error) {
      this.log(`No ${type} message found`, "warn");
      return null;
    }
  }



  /**
   * Wait for server to be ready
   */
  async waitForServer(maxAttempts = 30) {
    for (let i = 0; i < maxAttempts; i++) {
      try {
        const response = await this.page.goto("/_health", { timeout: 5000 });
        if (response && response.ok()) {
          this.log("Server is ready");
          return true;
        }
      } catch (error) {
        this.log(`Server check ${i + 1}/${maxAttempts} failed, retrying...`);
        await this.page.waitForTimeout(2000);
      }
    }
    throw new Error("Server failed to start within timeout");
  }

  /**
   * Create a form via UI
   */
  async createForm(formData) {
    this.log(`Creating form: ${formData.name}`);

    await this.navigateTo("/admin/forms/new");

    await this.fillForm({
      name: formData.name,
      slug: formData.slug,
    });

    await this.page.waitForLoadState("networkidle");
    this.log(`✅ Form created: ${formData.name}`);
  }

  /**
   * Submit to a form
   */
  async submitToForm(slug, token, data) {
    this.log(`Submitting to form: ${slug}`);

    const formData = new URLSearchParams();
    for (const [key, value] of Object.entries(data)) {
      formData.append(key, value);
    }

    const response = await this.page.request.post(
      `/forms/${slug}/submit?token=${token}`,
      {
        data: formData.toString(),
        headers: {
          "Content-Type": "application/x-www-form-urlencoded",
        },
      }
    );

    this.log(`Form submission response: ${response.status()}`);
    return response;
  }

  /**
   * Robust cleanup method
   */
  async cleanup() {
    try {
      // Close database connection
      await this.closeDB();

      // Clear browser state
      await this.page.context().clearCookies();
      await this.page.context().clearPermissions();
      this.log("Cleanup completed");
    } catch (error) {
      this.log(`Cleanup failed: ${error.message}`, "warn");
    }
  }
}

module.exports = { TestHelpers };
