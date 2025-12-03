# Formlander E2E Tests

End-to-end tests for Formlander using Playwright.

## Setup

Install Playwright and dependencies:

```bash
make test-e2e-setup
```

Or manually:

```bash
cd e2e
npm install
npx playwright install --with-deps chromium
```

## Running Tests

Run all E2E tests:

```bash
make test-e2e
```

Or manually:

```bash
cd e2e
npm test
```

## Test Structure

- **001-onboarding.spec.js** - MUST RUN FIRST - Sets up admin account
- **002-forms.spec.js** - Form CRUD operations
- **003-submissions.spec.js** - Form submissions and rate limiting
- **004-settings.spec.js** - Settings management (Mailgun, Turnstile)
- **099-logout.spec.js** - Final cleanup and logout

Tests run **sequentially** in numeric order due to dependencies.

## Default Admin Credentials

The app automatically creates a default admin on first run:

- Email: `admin@formlander.local`
- Password: `formlander`

The onboarding test changes these to:

- Email: `admin@test-e2e.com`
- Password: `testpassword123`

## Test Environment

Tests use `FORMLANDER_ENV=test` which creates a separate test database:

- Database: `storage/formlander-test.db`
- Server: `http://localhost:8080`

The test database is reset before each test run.

## Configuration

See `playwright.config.js` for Playwright configuration:

- Single worker (sequential execution)
- 45s timeout per test
- Screenshots on failure
- No videos (for speed)

## Debugging

Run with UI mode for debugging:

```bash
cd e2e
npx playwright test --ui
```

Run specific test file:

```bash
cd e2e
npx playwright test 003-submissions.spec.js
```

## CI/CD

The tests work in CI with `CI=true` environment variable:

- Retries enabled (1 retry)
- GitHub reporter
- Trace on first retry
