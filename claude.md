# Claude AI Assistant Instructions for Formlander

## Project Overview

Formlander is a self-hosted form backend written in Go. It uses Phoenix Context Architecture to organize code into bounded contexts with clear separation of concerns.

## Architecture Patterns

### Phoenix Context Architecture

Code is organized into contexts (bounded domains):
- **accounts** — User management
- **forms** — Form definitions and submissions
- **integrations** — External services (webhooks, Mailgun)
- **jobs** — Background processing
- **http** — HTTP handlers

Each context owns its domain logic and data access. Avoid cross-context direct database access.

### Cartridge Context Pattern

We use `internal/pkg/cartridge.Context` for request-scoped dependency injection:

```go
type Context struct {
    *fiber.Ctx
    Logger    *zap.Logger
    Config    *config.Config
    DBManager *database.Manager
}
```

**Important:** Access dependencies via fields, not `fiber.Ctx.Locals()`. Use `ctx.DB()` for database access.

### SQLite Write Handling

SQLite only allows **one writer** at a time. Always wrap write operations with `dbtxn.WithRetry`:

```go
import "formlander/internal/pkg/dbtxn"

err := dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
    return tx.Create(&record).Error
})
```

This handles:
- Busy/locked database errors
- Automatic retries with exponential backoff
- Jittered delays to prevent thundering herd

**Never** use raw `db.Create()` or `db.Save()` for writes without the retry wrapper.

## Code Style

- Use structured logging with `zap.Logger`
- Return errors, don't panic
- Prefer explicit over clever
- Comment only when clarification is needed
- Use Go formatting conventions (`gofmt`)

## Database Patterns

- GORM for ORM layer
- SQLite with WAL mode
- Transactions with immediate locks (`_txlock=immediate`)
- All writes via `dbtxn.WithRetry`
- Migrations in `internal/database/migrate.go`

## Testing

- Table-driven tests preferred
- Use `internal/pkg/testsupport` helpers
- In-memory SQLite for unit tests
- E2E tests in `e2e/` directory

## Common Tasks

### Adding a New Context

1. Create package under `internal/`
2. Define models (if needed)
3. Implement business logic with public API
4. Add tests
5. Update routing in `internal/routes.go`

### Adding a Database Write

Always use retry wrapper:

```go
err := dbtxn.WithRetry(ctx.Logger, db, func(tx *gorm.DB) error {
    // Your write operations here
    return tx.Create(&model).Error
})
```

### Adding a New Handler

```go
func HandleSomething(ctx *cartridge.Context) error {
    db, err := ctx.DB()
    if err != nil {
        return err
    }
    
    // Business logic...
    
    return ctx.JSON(fiber.Map{"success": true})
}
```

Register in `internal/routes.go`.

## Project Structure

```
internal/
├── accounts/        # User context
├── auth/           # Authentication
├── config/         # Configuration
├── database/       # DB manager & migrations
├── forms/          # Forms context
├── http/           # HTTP handlers
├── integrations/   # External services
├── jobs/           # Background jobs
└── pkg/
    ├── cartridge/  # Framework wrapper
    ├── dbtxn/      # Transaction helpers
    └── logger/     # Logging setup
```

## Key Files

- `internal/app.go` — Application bootstrap
- `internal/routes.go` — Route definitions
- `internal/database/manager.go` — Database connection pooling
- `internal/pkg/dbtxn/retry.go` — Write retry logic
- `internal/pkg/cartridge/context.go` — Request context

## License

MIT — This is an open-source project.
