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

```bash
docker run -d \
  -p 8080:8080 \
  -e FORMLANDER_SESSION_SECRET=$(openssl rand -hex 32) \
  -e FORMLANDER_ANON_SALT=$(openssl rand -hex 32) \
  -v $(pwd)/storage:/app/storage \
  karloscodes/formlander:latest
```

**Required Environment Variables:**
- `FORMLANDER_SESSION_SECRET` - HMAC secret for signing session cookies (generate with `openssl rand -hex 32`)
- `FORMLANDER_ANON_SALT` - Salt for hashing IP addresses (generate with `openssl rand -hex 32`)

Access the admin dashboard at `http://localhost:8080` with default credentials:
- Email: `admin@formlander.local`
- Password: `formlander` (you'll be prompted to change this on first login)

### Running the Binary

1. Download the latest release from the [Releases page](https://github.com/karloscodes/formlander/releases)
2. Extract and set required environment variables:
   ```bash
   export FORMLANDER_SESSION_SECRET=$(openssl rand -hex 32)
   export FORMLANDER_ANON_SALT=$(openssl rand -hex 32)
   export FORMLANDER_DATA_DIR=./storage
   ```
3. Run the binary:
   ```bash
   ./formlander
   ```

**Optional Environment Variables:**
- `FORMLANDER_ENV` - Environment mode (default: `production`)
- `FORMLANDER_PORT` - HTTP port (default: `8080`)
- `FORMLANDER_LOG_LEVEL` - Log level: `debug`, `info`, `warn`, `error` (default: `info`)
- `FORMLANDER_DATA_DIR` - Data directory path (default: `./storage`)
- `FORMLANDER_DATABASE_FILENAME` - Database filename (default: `formlander.db`)

### Building from Source

1. Clone the repository
2. Build:
   ```bash
   make build
   ```
3. Set environment variables and run:
   ```bash
   export FORMLANDER_SESSION_SECRET=$(openssl rand -hex 32)
   export FORMLANDER_ANON_SALT=$(openssl rand -hex 32)
   ./bin/formlander
   ```

## Releases

Formlander uses semantic versioning. Docker images and binaries are published via GitHub Releases when version tags are pushed.

### Docker Images

Once the first release is published, images will be available at:

```bash
# Latest stable release
docker pull karloscodes/formlander:latest

# Specific version
docker pull karloscodes/formlander:v1.0.0

# Major version (receives minor + patch updates)
docker pull karloscodes/formlander:v1
```

**Note:** Docker images are published automatically via GitHub Actions when a version tag (e.g., `v1.0.0`) is pushed to the repository.

### Binaries

Download platform-specific binaries from the [Releases page](https://github.com/karloscodes/formlander/releases).

Supported platforms:
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

MIT License — see [LICENSE](LICENSE) file for details.
