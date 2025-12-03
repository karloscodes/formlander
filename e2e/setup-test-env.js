const { execSync } = require("node:child_process");
const path = require("node:path");
const fs = require("node:fs");

const projectRoot = path.resolve(__dirname, "..");
const DEFAULT_DB_FILENAME = "formlander.db";
const browsersCacheDir =
  process.env.PLAYWRIGHT_BROWSERS_PATH ||
  path.join(projectRoot, "tmp", "ms-playwright");

// Ensure all commands reuse the local browser cache inside the repo so we don't
// need write access to ~/Library/Caches.
process.env.PLAYWRIGHT_BROWSERS_PATH = browsersCacheDir;

// Ensure we're using the test environment
process.env.FORMLANDER_ENV = "test";

console.log("=== E2E Test Environment Setup ===");
console.log(`Environment: ${process.env.FORMLANDER_ENV}`);
console.log(`Node version: ${process.version}`);
console.log(`Platform: ${process.platform}`);

// Helper function to run commands with better error handling
function runCommand(command, description, options = {}) {
  console.log(`\n📋 ${description}...`);
  try {
    const result = execSync(command, {
      stdio: "inherit",
      encoding: "utf8",
      ...options,
    });
    console.log(`✅ ${description} completed successfully`);
    return result;
  } catch (error) {
    console.error(`❌ ${description} failed:`);
    console.error(`Command: ${command}`);
    console.error(`Exit code: ${error.status}`);
    console.error(`Error: ${error.message}`);
    throw error;
  }
}

// Helper function to check if a file exists
function checkFileExists(filePath, description) {
  if (fs.existsSync(filePath)) {
    console.log(`✅ ${description} exists: ${filePath}`);
    return true;
  } else {
    console.log(`❌ ${description} missing: ${filePath}`);
    return false;
  }
}

function ensureDirectory(dirPath, label) {
  if (!fs.existsSync(dirPath)) {
    console.log(`📁 Creating ${label} directory at ${dirPath}`);
    fs.mkdirSync(dirPath, { recursive: true });
  }
}

function resolveDataDirectory() {
  const configured = process.env.FORMLANDER_DATA_DIR;
  if (!configured) {
    return path.join(projectRoot, "storage");
  }
  return path.isAbsolute(configured)
    ? configured
    : path.join(projectRoot, configured);
}

function resolveDatabasePaths() {
  const envName = (process.env.FORMLANDER_ENV || "development").toLowerCase();
  let filename = process.env.FORMLANDER_DATABASE_FILENAME || DEFAULT_DB_FILENAME;
  const override = process.env.FORMLANDER_DATABASE_PATH;

  let databasePath = override;
  if (!databasePath) {
    if (filename.toLowerCase() === DEFAULT_DB_FILENAME) {
      const ext = path.extname(filename) || ".db";
      const base = path.basename(filename, ext);
      filename = `${base}.${envName}${ext}`;
    }
    databasePath = filename;
  }

  if (!path.isAbsolute(databasePath)) {
    databasePath = path.join(resolveDataDirectory(), databasePath);
  }

  return {
    dbPath: databasePath,
    walPath: `${databasePath}-wal`,
    shmPath: `${databasePath}-shm`,
  };
}

function ensurePlaywrightBrowsers() {
  ensureDirectory(browsersCacheDir, "Playwright browsers");

  const hasChromium = fs
    .readdirSync(browsersCacheDir)
    .some((entry) => entry.startsWith("chromium-"));

  if (hasChromium) {
    console.log(`✅ Playwright Chromium detected at ${browsersCacheDir}`);
    return;
  }

  runCommand(
    "npx playwright install chromium",
    "Installing Playwright Chromium browser",
    {
      cwd: __dirname,
      env: {
        ...process.env,
        PLAYWRIGHT_BROWSERS_PATH: browsersCacheDir,
      },
    }
  );
}

async function setupTestEnvironment() {
  try {
    console.log(`📁 Project root: ${projectRoot}`);

    ensureDirectory(path.join(projectRoot, "tmp"), "tmp");
    ensureDirectory(path.join(projectRoot, "tmp", "go-cache"), "go cache");
    ensureDirectory(browsersCacheDir, "Playwright browsers");

    // Validate project structure
    const requiredPaths = [
      path.join(projectRoot, "go.mod"),
      path.join(projectRoot, "cmd/formlander/main.go"),
      path.join(projectRoot, "Makefile"),
    ];

    for (const requiredPath of requiredPaths) {
      if (!checkFileExists(requiredPath, `Required file ${path.basename(requiredPath)}`)) {
        throw new Error(`Missing required file: ${requiredPath}`);
      }
    }

    // Ensure storage directory exists
    const dataDir = resolveDataDirectory();
    ensureDirectory(dataDir, "storage");

    // Note: Database cleanup is handled by the webServer command in playwright.config.js
    // to ensure clean state before the server starts

    // Build the binary for testing
    runCommand(
      "make build",
      "Building formlander binary",
      {
        env: { ...process.env, FORMLANDER_ENV: "test" },
        cwd: projectRoot,
      }
    );

    runCommand(
      `GOCACHE=${path.join(projectRoot, "tmp", "go-cache")} FORMLANDER_ENV=test LOG_LEVEL=error go run cmd/formlander/main.go --help || echo "Binary ready for testing"`,
      "Verifying binary is ready",
      {
        cwd: projectRoot,
        env: {
          ...process.env,
          FORMLANDER_ENV: "test",
          LOG_LEVEL: "error",
          GOCACHE: path.join(projectRoot, "tmp", "go-cache"),
        },
      }
    );

    ensurePlaywrightBrowsers();

    console.log("\n=== ✅ Test Environment Setup Complete ===");
    console.log("The Playwright webServer will start the application automatically");
    console.log("Tests will create their own data including admin user:");
    console.log("  Email: admin@formlander.local");
    console.log("  Password: formlander");
    console.log("No seed data - tests are fully independent");
    console.log("=============================================\n");

  } catch (error) {
    console.error("\n=== ❌ Test Environment Setup Failed ===");
    console.error(error.message);
    console.error("Please check the error messages above and fix the issues.");
    process.exit(1);
  }
}

// Export for Playwright globalSetup
module.exports = setupTestEnvironment;

// If running directly
if (require.main === module) {
  setupTestEnvironment().catch((error) => {
    console.error(error);
    process.exit(1);
  });
}
