# HTTP Routing Pattern

## Overview

Formlander uses the cartridge framework (Fiber wrapper) with config-based route definitions.

## Route Configuration

Location: `internal/routes.go`

```go
func (s *Server) setupRoutes() {
    // Auth config with session middleware
    authConfig := &cartridge.RouteConfig{
        CustomMiddleware: []fiber.Handler{
            s.Session().Middleware(),
            httphandlers.RequirePasswordChanged(),
        },
    }

    // Public config with rate limiting and CORS
    publicConfig := &cartridge.RouteConfig{
        EnableSecFetchSite: cartridge.Bool(false),
        EnableCORS:         true,
        CORSConfig: &cors.Config{
            AllowOrigins: "*",
            AllowMethods: "POST,OPTIONS",
            AllowHeaders: "Content-Type, Authorization",
        },
        WriteConcurrency: true,
        CustomMiddleware: []fiber.Handler{
            limiter.New(limiter.Config{
                Max:        30,
                Expiration: 60 * time.Second,
            }),
        },
    }

    // Define routes
    s.Get("/admin/forms", httphandlers.AdminFormsIndex, authConfig)
    s.Post("/forms/:slug/submit", httphandlers.PublicFormSubmission, publicConfig)
}
```

## Handler Signature

All handlers use `cartridge.Context`:

```go
func AdminFormsIndex(ctx *cartridge.Context) error {
    db := ctx.DB()
    user := ctx.User()
    logger := ctx.Logger()

    forms, err := forms.List(db)
    if err != nil {
        return fiber.ErrInternalServerError
    }

    return ctx.Render("layouts/base", fiber.Map{
        "Title":       "Forms",
        "Forms":       forms,
        "ContentView": "admin/forms/index/content",
    }, "")
}
```

## Route Config Options

```go
&cartridge.RouteConfig{
    // Middleware
    CustomMiddleware: []fiber.Handler{...},

    // CORS
    EnableCORS: true,
    CORSConfig: &cors.Config{...},

    // Security
    EnableSecFetchSite: cartridge.Bool(false),

    // Concurrency
    WriteConcurrency: true,  // Enable write semaphore limiting
}
```

## Response Types

```go
// JSON response
return ctx.JSON(fiber.Map{
    "success": true,
    "data":    result,
})

// HTML template
return ctx.Render("layouts/base", fiber.Map{
    "Title":       "Page Title",
    "ContentView": "path/to/content",
}, "")

// Redirect
return ctx.Redirect("/admin/forms")

// Status codes
return ctx.Status(201).JSON(fiber.Map{...})
return ctx.SendStatus(204)  // No content
```

## URL Parameters

```go
// Path params: /forms/:id
id := ctx.Params("id")

// Query params: /forms?page=1
page := ctx.Query("page", "1")  // with default

// Form data
name := ctx.FormValue("name")
```

## Rate Limiting

Applied per-route via middleware:

```go
limiter.New(limiter.Config{
    Max:        30,              // requests
    Expiration: 60 * time.Second, // per minute
    KeyGenerator: func(c *fiber.Ctx) string {
        return c.IP()
    },
    Next: func(c *fiber.Ctx) bool {
        return cfg.IsDevelopment()  // skip in dev
    },
})
```

## Write Concurrency Limiting

For SQLite write protection:

```go
publicConfig := &cartridge.RouteConfig{
    WriteConcurrency: true,  // Enables semaphore-based limiting
}
```
