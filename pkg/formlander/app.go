// Package formlander provides a public API for extending Formlander
package formlander

import (
	"time"

	"formlander/internal"
	"formlander/internal/accounts"
	"formlander/internal/auth"
	"formlander/internal/database"
	"formlander/internal/pkg/cartridge"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Context is the public alias for cartridge.Context
type Context = cartridge.Context

// Server is the public alias for cartridge.Server
type Server = cartridge.Server

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

// GetServer returns the cartridge server for registering routes with context
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
	return auth.GetUserID(ctx)
}
