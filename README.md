# Formlander

A self-hosted drop-in backend for HTML forms. Accept, store, review, and route form submissions without surrendering control to third-party SaaS providers.

🌐 **[formlander.com](https://formlander.com)**

## Overview

Formlander enables developers running static or serverless sites to handle form submissions with a single-binary deployment. Store submissions in SQLite, review them in a lightweight admin UI, and route data asynchronously to webhooks or email via Mailgun.

## Features

- **Single-binary deployment** — One executable with SQLite storage, no external dependencies
- **Admin dashboard** — View and manage forms, submissions, and delivery status
- **Asynchronous delivery** — Queue webhook and email notifications with retry logic
- **Spam protection** — Configurable honeypot fields and rate limiting
- **API-first design** — Dashboard consumes the same REST endpoints available for integrations
- **Privacy-focused** — All data stored locally; optional Mailgun integration for email forwarding

## Quick Start

### Using Docker (Recommended)

**First, generate and save your session secret:**

```bash
# Generate once and save this value securely
export FORMLANDER_SESSION_SECRET=$(openssl rand -hex 32)
echo "Save this secret: $FORMLANDER_SESSION_SECRET"
```

**Then run the container with your saved secret:**

```bash
docker run -d \
  -p 8080:8080 \
  -e FORMLANDER_SESSION_SECRET="your-saved-secret-here" \
  -v $(pwd)/storage:/app/storage \
  karloscodes/formlander:latest
```

**Important:** Use the same `FORMLANDER_SESSION_SECRET` value across restarts to prevent logging out all users.

Access the admin dashboard at `http://localhost:8080` with default credentials:
- Email: `admin@formlander.local`
- Password: `formlander` (you'll be prompted to change this on first login)

### Running the Binary

1. Download the latest release from the [Releases page](https://github.com/karloscodes/formlander/releases)
2. Generate and save your session secret:
   ```bash
   # Generate once and save this value
   export FORMLANDER_SESSION_SECRET=$(openssl rand -hex 32)
   echo "Save this secret: $FORMLANDER_SESSION_SECRET"
   
   export FORMLANDER_DATA_DIR=./storage
   ```
3. Run the binary:
   ```bash
   ./formlander
   ```

## Configuration

Formlander uses [Viper](https://github.com/spf13/viper) for flexible configuration. You can configure via:
- Environment variables (prefix: `FORMLANDER_`)
- `.env` file for easier local development
- Environment variables always override `.env` file values

**Required Environment Variable:**
- `FORMLANDER_SESSION_SECRET` - HMAC secret for signing session cookies (must persist across restarts)

**Optional Environment Variables:**
- `FORMLANDER_ENV` - Environment mode: `development`, `production` (default: `development`)
- `FORMLANDER_PORT` - HTTP port (default: `8080`)
- `FORMLANDER_LOG_LEVEL` - Log level: `debug`, `info`, `warn`, `error` (default: `info`)
- `FORMLANDER_DATA_DIR` - Data directory path (default: `./storage`)

**Or use a .env file** (`.env`):
```bash
FORMLANDER_ENV=production
FORMLANDER_PORT=8080
FORMLANDER_SESSION_SECRET=your-secret-here
FORMLANDER_LOG_LEVEL=info
FORMLANDER_DATA_DIR=./storage
```

### Building from Source

1. Clone the repository
2. Build:
   ```bash
   make build
   ```
3. Generate and save your session secret, then run:
   ```bash
   # Generate once and save this value
   export FORMLANDER_SESSION_SECRET=$(openssl rand -hex 32)
   echo "Save this secret: $FORMLANDER_SESSION_SECRET"
   
   ./bin/formlander
   ```

## Releases

Formlander uses semantic versioning. Docker images are published via GitHub Releases when version tags are pushed.

### Docker Images

```bash
# Latest stable release
docker pull karloscodes/formlander:latest

# Specific version
docker pull karloscodes/formlander:v1.0.0

# Major version (receives minor + patch updates)
docker pull karloscodes/formlander:v1
```

**Note:** Docker images are published automatically via GitHub Actions when a version tag (e.g., `v1.0.0`) is pushed to the repository.

### Building from Source

If you prefer to run a native binary instead of Docker:

```bash
git clone https://github.com/karloscodes/formlander.git
cd formlander
make build
export FORMLANDER_SESSION_SECRET=$(openssl rand -hex 32)
./bin/formlander
```

**Supported platforms for building from source:**
- Linux (amd64, arm64)
- macOS (amd64, arm64)

## Architecture

Formlander follows a **Phoenix Context Architecture**, organizing code into bounded contexts with clear separation of concerns:

```
[Static Site] --> POST /forms/:slug/submit
                        |
                  [Formlander]
                   /    |    \
            [SQLite] [Jobs] [Admin UI]
                      |
               [Webhook/Email Dispatchers]
                      |
            [External Services/Mailgun]
```

### Key Components

- **HTTP Server** — Fiber-based cartridge wrapper handling public submissions and admin dashboard
- **Database Layer** — GORM + SQLite with WAL mode
- **Custom Write Retry Logic** — `dbtxn.WithRetry` ensures writes eventually succeed despite SQLite's single-writer constraint
- **Job System** — In-process dispatchers for asynchronous webhook and email delivery
- **Cartridge Context** — Request-scoped dependency injection providing type-safe access to logger, config, and database

### SQLite Write Handling

Due to SQLite's single-writer limitation, all write operations use a custom retry mechanism (`internal/pkg/dbtxn/retry.go`) that:
- Detects busy/locked database errors
- Retries with exponential backoff (up to 10 attempts)
- Adds jitter to prevent thundering herd issues
- Works alongside WAL mode, busy_timeout pragmas, and immediate transaction locks

This ensures writes eventually succeed even under concurrent load.

## Development

Build and run locally:
```bash
make build
make dev
```

Run tests:
```bash
make test
```

## Contributing

Contributions are welcome! Please open an issue first to discuss proposed changes, or submit a pull request for bug fixes and improvements.

## License

Formlander License Agreement — see [LICENSE](LICENSE) file for details.
