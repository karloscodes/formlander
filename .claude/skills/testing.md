# Testing Pattern

## Overview

Formlander uses testify assertions with a shared test database helper. Tests are organized using subtests (contexts) for clarity. Table-driven tests are used only when testing many similar input/output variations.

## Test Database Setup

Location: `internal/pkg/testsupport/db.go`

```go
import "formlander/internal/pkg/testsupport"

func TestCreateForm(t *testing.T) {
    db := testsupport.SetupTestDB(t)
    logger := slog.New(slog.NewTextHandler(io.Discard, nil))

    // Test code here
}
```

## Subtests (Primary Pattern)

Use `t.Run()` to organize tests into contexts:

```go
func TestFormOperations(t *testing.T) {
    db := testsupport.SetupTestDB(t)
    logger := slog.New(slog.NewTextHandler(io.Discard, nil))

    t.Run("Create", func(t *testing.T) {
        form := &forms.Form{Name: "Test Form"}
        err := forms.Create(logger, db, form)
        require.NoError(t, err)
        assert.NotZero(t, form.ID)
        assert.NotEmpty(t, form.PublicID)
    })

    t.Run("GetByID", func(t *testing.T) {
        form := createTestForm(t, db, "Lookup Test")
        result, err := forms.GetByID(db, form.ID)
        require.NoError(t, err)
        assert.Equal(t, form.Name, result.Name)
    })

    t.Run("Delete", func(t *testing.T) {
        form := createTestForm(t, db, "Delete Test")
        err := forms.Delete(logger, db, form.ID)
        require.NoError(t, err)

        _, err = forms.GetByID(db, form.ID)
        assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
    })
}
```

## Assertions

Use testify for assertions:

```go
import (
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestExample(t *testing.T) {
    // Use require for fatal checks (test stops if fails)
    require.NoError(t, err)
    require.NotNil(t, result)

    // Use assert for non-fatal checks (test continues)
    assert.Equal(t, expected, actual)
    assert.Contains(t, str, "substring")
    assert.True(t, condition)
    assert.ErrorIs(t, err, expectedErr)
}
```

## Test Helpers

Create helper functions for common setup:

```go
func createTestUser(t *testing.T, db *gorm.DB, email, password string) *accounts.User {
    hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    require.NoError(t, err)

    user := &accounts.User{
        Email:        email,
        PasswordHash: string(hash),
    }
    require.NoError(t, db.Create(user).Error)
    return user
}

func createTestForm(t *testing.T, db *gorm.DB, name string) *forms.Form {
    form := &forms.Form{Name: name}
    require.NoError(t, db.Create(form).Error)
    return form
}
```

## Table-Driven Tests (When Appropriate)

Use table-driven tests only for testing many similar variations of input/output:

```go
func TestIsBusyError(t *testing.T) {
    tests := []struct {
        name string
        err  error
        want bool
    }{
        {"nil error", nil, false},
        {"database is locked", errors.New("database is locked"), true},
        {"database is busy", errors.New("database is busy"), true},
        {"unrelated error", errors.New("connection refused"), false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := isBusyError(tt.err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

**When to use table-driven:**
- Testing a function with many input variations
- Validation functions with multiple edge cases
- Pure functions without side effects

**When NOT to use table-driven:**
- Tests with complex setup/teardown
- Tests requiring database state
- Integration tests

## Silent Logging in Tests

```go
logger := slog.New(slog.NewTextHandler(io.Discard, nil))
```

## Running Tests

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific package
go test ./internal/forms/...

# Run specific test
go test -v -run TestFormOperations ./internal/forms/...
```
