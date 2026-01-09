# Architecture Pattern

## Overview

Formlander follows a domain-driven layered architecture similar to Phoenix Contexts but adapted for Go.

## Directory Structure

```
internal/
├── forms/           # Forms domain (models + business logic)
├── accounts/        # User accounts domain
├── integrations/    # Third-party integrations (Mailgun, Turnstile)
├── jobs/            # Background job processors
├── http/            # HTTP handlers (cartridge-based)
├── middleware/      # HTTP middleware
├── auth/            # Session management
├── config/          # Configuration
├── database/        # Migrations and DB setup
├── server/          # Server setup and error handling
├── installer/       # Installation management
└── pkg/             # Shared utilities (dbtxn, testsupport)
```

## Layer Responsibilities

### 1. Models Layer (`internal/{domain}/models.go`)

GORM models with validation and hooks:

```go
type Form struct {
    gorm.Model
    PublicID string `gorm:"uniqueIndex;size:12"`
    Name     string `gorm:"not null"`
    Slug     string `gorm:"uniqueIndex"`
}

func (f *Form) BeforeCreate(tx *gorm.DB) error {
    if f.PublicID == "" {
        f.PublicID = GeneratePublicID()
    }
    return nil
}
```

### 2. Business Logic Layer (`internal/{domain}/{domain}.go`)

Service functions that handle transactions and business rules:

```go
// internal/forms/forms.go
func Create(logger *slog.Logger, db *gorm.DB, form *Form) error {
    return dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
        return tx.Create(form).Error
    })
}

func GetByID(db *gorm.DB, id uint) (*Form, error) {
    var form Form
    if err := db.Preload("WebhookDelivery").First(&form, id).Error; err != nil {
        return nil, err
    }
    return &form, nil
}
```

### 3. HTTP Handler Layer (`internal/http/*.go`)

Handlers receive cartridge.Context and delegate to business logic:

```go
// internal/http/forms.go
func AdminFormsIndex(ctx *cartridge.Context) error {
    db := ctx.DB()
    formsList, err := forms.List(db)
    if err != nil {
        return fiber.ErrInternalServerError
    }
    return ctx.Render("layouts/base", fiber.Map{
        "Title": "Forms",
        "Forms": formsList,
        "ContentView": "admin/forms/index/content",
    }, "")
}
```

### 4. Routes (`internal/routes.go`)

Route definitions with middleware configuration:

```go
func (s *Server) setupRoutes() {
    authConfig := &cartridge.RouteConfig{
        CustomMiddleware: []fiber.Handler{s.Session().Middleware()},
    }

    s.Get("/admin/forms", httphandlers.AdminFormsIndex, authConfig)
    s.Post("/admin/forms", httphandlers.AdminFormsCreate, authConfig)
}
```

## Key Principles

1. **Domain isolation**: Each domain (forms, accounts) owns its models and business logic
2. **Transaction handling**: Business layer handles transactions with retry logic
3. **Thin handlers**: HTTP handlers only parse input and call business functions
4. **Preloading**: Explicitly preload associations when needed
5. **Error types**: Each domain defines its own error types
