// Package formlander provides a public API for extending Formlander
package formlander

import (
	"time"

	"formlander/internal"
	"formlander/internal/accounts"
	"formlander/internal/auth"
	"formlander/internal/database"
	httphandlers "formlander/internal/http"
	"formlander/internal/server"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Context is the public alias for server.Context
type Context = server.Context

// Server is the public alias for server.Server
type Server = server.Server

// RouteConfig is the public alias for server.RouteConfig
type RouteConfig = server.RouteConfig

// App wraps the internal application with a public API
type App struct {
	internal *internal.App
}

// AppOptions configures formlander application initialization
type AppOptions struct {
	TemplatesDirectory string // Optional: custom template directory for development
}

// NewApp creates a new Formlander application
func NewApp() (*App, error) {
	return NewAppWithOptions(nil)
}

// NewAppWithOptions creates a new Formlander application with custom options
func NewAppWithOptions(opts *AppOptions) (*App, error) {
	app, err := internal.NewAppWithOptions(&internal.AppOptions{
		TemplatesDirectory: getTemplatesDirectory(opts),
	})
	if err != nil {
		return nil, err
	}

	return &App{internal: app}, nil
}

func getTemplatesDirectory(opts *AppOptions) string {
	if opts != nil && opts.TemplatesDirectory != "" {
		return opts.TemplatesDirectory
	}
	return ""
}

// GetFiber returns the underlying Fiber app for adding routes
func (a *App) GetFiber() *fiber.App {
	return a.internal.Server.App()
}

// GetServer returns the server for registering routes with context
func (a *App) GetServer() *server.Server {
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
	return auth.GetUserID(ctx)
}

// AuthMiddleware returns the authentication middleware
func AuthMiddleware() fiber.Handler {
	return auth.Middleware()
}

// RequirePasswordChangedMiddleware returns middleware that enforces password change
func RequirePasswordChangedMiddleware() fiber.Handler {
	return httphandlers.RequirePasswordChanged()
}
