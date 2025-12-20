// Package formlander provides a public API for extending Formlander
package formlander

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"
	"gorm.io/gorm"

	"formlander/internal"
	"formlander/internal/accounts"
	"formlander/internal/database"
	httphandlers "formlander/internal/http"
)

// Context is the public alias for cartridge.Context
type Context = cartridge.Context

// Server is the public alias for cartridge.Server
type Server = cartridge.Server

// RouteConfig is the public alias for cartridge.RouteConfig
type RouteConfig = cartridge.RouteConfig

// App wraps the internal application with a public API
type App struct {
	internal *internal.App
}

// NewApp creates a new Formlander application
func NewApp() (*App, error) {
	app, err := internal.NewApp()
	if err != nil {
		return nil, err
	}
	return &App{internal: app}, nil
}

// GetFiber returns the underlying Fiber app for adding routes
func (a *App) GetFiber() *fiber.App {
	return a.internal.Server.App()
}

// GetServer returns the server for registering routes with context
func (a *App) GetServer() *cartridge.Server {
	return a.internal.Server
}

// GetDB returns the database connection
func (a *App) GetDB() *gorm.DB {
	return a.internal.GetDB()
}

// RunMigrations runs database migrations
func (a *App) RunMigrations() error {
	return internal.RunMigrations(a.internal)
}

// RunWithTimeout starts the app with graceful shutdown
func (a *App) RunWithTimeout(timeout time.Duration) error {
	return a.internal.RunWithTimeout(timeout)
}

// Seed seeds the database with sample data
func Seed(db *gorm.DB) error {
	return database.Seed(db)
}

// FindUserByID finds a user by ID
func FindUserByID(db *gorm.DB, id uint) (*accounts.User, error) {
	return accounts.FindByID(db, id)
}

// GetUserID retrieves the current user ID from context
func GetUserID(ctx *fiber.Ctx) (uint, bool) {
	session := httphandlers.GetSessionFromFiber(ctx)
	if session == nil {
		return 0, false
	}
	return session.GetUserID(ctx)
}

// AuthMiddleware returns the authentication middleware
func AuthMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		session := httphandlers.GetSessionFromFiber(c)
		if session == nil {
			return c.Redirect("/admin/login")
		}
		return session.Middleware()(c)
	}
}

// RequirePasswordChangedMiddleware returns middleware that enforces password change
func RequirePasswordChangedMiddleware() fiber.Handler {
	return httphandlers.RequirePasswordChanged()
}
