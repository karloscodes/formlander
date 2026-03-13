# SQLite Write Retry Pattern

## Overview

All database write operations in Formlander MUST use the `dbtxn.WithRetry()` wrapper to handle SQLite busy errors gracefully.

## Location

`internal/pkg/dbtxn/retry.go`

## Usage

```go
import "formlander/internal/pkg/dbtxn"

// Correct - all writes use WithRetry
err := dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
    return tx.Create(&model).Error
})

// Wrong - direct writes without retry
err := db.Create(&model).Error  // DON'T DO THIS
```

## Configuration

- **Max retries**: 10 attempts
- **Base delay**: 100ms
- **Max delay**: 5 seconds
- **Jitter**: 20% random variance to prevent thundering herd

## Busy Error Detection

The retry logic catches these SQLite errors:
- `database is locked`
- `database is busy`
- `database table is locked`
- `SQL statements in progress`

## Example

```go
func CreateForm(logger *slog.Logger, db *gorm.DB, form *Form) error {
    return dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
        return tx.Create(form).Error
    })
}

func UpdateSubmission(logger *slog.Logger, db *gorm.DB, id uint, data map[string]any) error {
    return dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
        return tx.Model(&Submission{}).Where("id = ?", id).Updates(data).Error
    })
}
```

## When to Use

- Creating records
- Updating records
- Deleting records
- Any transaction that modifies data

## When NOT to Use

- Read-only queries (SELECT)
- Migrations (handled separately)
