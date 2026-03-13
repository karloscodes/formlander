# Error Handling Pattern

## Overview

Formlander uses domain-specific error types and a global error handler for consistent error responses.

## Domain Error Types

Each domain defines its own sentinel errors:

```go
// internal/accounts/accounts.go
var (
    ErrInvalidCredentials = errors.New("invalid email or password")
    ErrUserNotFound       = errors.New("user not found")
    ErrWeakPassword       = errors.New("password must be at least 8 characters")
    ErrPasswordMismatch   = errors.New("current password is incorrect")
)

// internal/forms/forms.go
type ValidationError struct {
    Field   string
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("%s: %s", e.Field, e.Message)
}
```

## Error Checking

Use `errors.Is()` for sentinel errors:

```go
if errors.Is(err, accounts.ErrInvalidCredentials) {
    return ctx.Status(401).JSON(fiber.Map{"error": "Invalid credentials"})
}

if errors.Is(err, gorm.ErrRecordNotFound) {
    return fiber.ErrNotFound
}
```

## Global Error Handler

Location: `internal/server/server.go`

```go
func ErrorHandler(log *slog.Logger, cfg *config.Config) fiber.ErrorHandler {
    return func(c *fiber.Ctx, err error) error {
        code := fiber.StatusInternalServerError
        if e, ok := err.(*fiber.Error); ok {
            code = e.Code
        }

        log.Error("request failed",
            slog.Any("error", err),
            slog.String("path", c.Path()),
            slog.Int("status", code),
        )

        // JSON for API requests
        if c.Accepts(fiber.MIMEApplicationJSON) == fiber.MIMEApplicationJSON {
            return c.Status(code).JSON(fiber.Map{
                "error": "internal_server_error",
                "message": err.Error(),
            })
        }

        // HTML for browser requests
        return c.Status(code).Render("layouts/base", fiber.Map{
            "Title": "Error",
            "ContentView": "errors/500/content",
        }, "")
    }
}
```

## HTTP Error Responses

Use Fiber's built-in errors for common cases:

```go
return fiber.ErrNotFound           // 404
return fiber.ErrBadRequest         // 400
return fiber.ErrUnauthorized       // 401
return fiber.ErrForbidden          // 403
return fiber.ErrInternalServerError // 500
```

## Logging Errors

Always log with context:

```go
logger.Error("failed to create form",
    slog.Any("error", err),
    slog.String("form_name", form.Name),
    slog.Uint64("user_id", uint64(userID)),
)
```
